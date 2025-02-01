package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/noboruma/s1h/internal/credentials"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/rivo/tview"

	cssh "golang.org/x/crypto/ssh"
)

var (
	autoCompleteIPs   []string
	autoCompleteHosts []string
)

func executeSSHShell(endpoint, user, password, identityFile string) {

	var config cssh.ClientConfig

	if password != "" {
		config = cssh.ClientConfig{
			User: user,
			Auth: []cssh.AuthMethod{
				cssh.Password(password),
			},
			HostKeyCallback: cssh.InsecureIgnoreHostKey(),
		}
	} else if identityFile != "" {
		auth, err := ssh.LoadIdentifyFile(identityFile)
		if err != nil {
			println(err.Error())
			return
		}
		config = cssh.ClientConfig{
			User: user,
			Auth: []cssh.AuthMethod{
				cssh.PublicKeys(auth),
			},
			HostKeyCallback: cssh.InsecureIgnoreHostKey(),
		}
	}

	client, err := cssh.Dial("tcp", endpoint, &config)
	if err != nil {
		log.Fatal(err)
	}

	session, err := client.NewSession()
	if err != nil {
		log.Fatal(err)
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	err = session.RequestPty("xterm-256color", 80, 40, cssh.TerminalModes{})
	if err != nil {
		log.Fatalf("failed to request PTY: %v", err)
	}

	err = session.Shell()
	if err != nil {
		log.Fatal(err)
	}

	_ = session.Wait()
}

func displaySSHConfig(configs []ssh.SSHConfig, creds credentials.Credentials) {
	app := tview.NewApplication()

	sshOutput := tview.NewTextView().
		SetText("Connecting...\n").
		SetTextAlign(tview.AlignLeft).
		SetDynamicColors(true)

	sshPage := tview.NewFlex().
		SetDirection(tview.FlexRow).
		SetFullScreen(true).
		AddItem(sshOutput, 0, 1, false)

	pages := tview.NewPages()

	table := tview.NewTable().
		SetBorders(false)

	table.SetSelectable(true, false)

	table.SetCell(0, 0, tview.NewTableCell("Host (F1)").
		SetTextColor(tcell.ColorBlue).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	table.SetCell(0, 1, tview.NewTableCell("User (F2)").
		SetTextColor(tcell.ColorBlueViolet).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	table.SetCell(0, 2, tview.NewTableCell("Port (F3)").
		SetTextColor(tcell.ColorGreen).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	table.SetCell(0, 3, tview.NewTableCell("HostName (F4)").
		SetTextColor(tcell.ColorRed).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	table.SetCell(0, 4, tview.NewTableCell("IdentityFile (F5)").
		SetTextColor(tcell.ColorBlue).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	table.SetCell(0, 5, tview.NewTableCell("Password").
		SetTextColor(tcell.ColorBlue).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))

	for i, config := range configs {
		table.SetCell(i+1, 0, tview.NewTableCell(config.Host).
			SetAlign(tview.AlignLeft))
		table.SetCell(i+1, 1, tview.NewTableCell(config.User).
			SetAlign(tview.AlignLeft))
		table.SetCell(i+1, 2, tview.NewTableCell(config.Port).
			SetAlign(tview.AlignLeft))
		table.SetCell(i+1, 3, tview.NewTableCell(config.HostName).
			SetAlign(tview.AlignLeft))
		table.SetCell(i+1, 4, tview.NewTableCell(config.IdentityFile).
			SetAlign(tview.AlignLeft))
		if _, has := creds.Entries[config.Host]; has {
			table.SetCell(i+1, 5, tview.NewTableCell("O").
				SetAlign(tview.AlignLeft))
		}
	}

	table.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorViolet))
	table.SetSelectedFunc(func(row, column int) {
		selectedConfig := configs[row-1]
		if _, has := creds.Entries[selectedConfig.Host]; !has && selectedConfig.IdentityFile == "" {
			popup := tview.NewModal().
				SetText(fmt.Sprintf("Missing credentials for Host: %s\nUser: %s\nPort: %s\nHostName: %s\nIdentityFile: %s",
					selectedConfig.Host, selectedConfig.User, selectedConfig.Port, selectedConfig.HostName, selectedConfig.IdentityFile)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					pages.RemovePage("popup")
				})
			pages.AddPage("popup", popup, false, true)
			return
		}

		pages.AddPage("popup", sshPage, true, true)
		app.Suspend(func() {
			executeSSHShell(fmt.Sprintf("%s:%s", selectedConfig.HostName, selectedConfig.Port), selectedConfig.User, creds.Entries[selectedConfig.Host], selectedConfig.IdentityFile)
			pages.RemovePage("popup")
		})

	})

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			if pages.HasPage("popup") {
				pages.RemovePage("popup")
			} else {
				app.Stop()
			}
		case tcell.KeyF1:
			popup := tview.NewInputField()
			popup.SetLabel("Search IP: ").SetDoneFunc(func(buttonIndex tcell.Key) {
				ip := ""
				if buttonIndex == tcell.KeyEnter {
					ip = popup.GetText()
				}
				if ip != "" {
					for i, cfg := range configs {
						if cfg.HostName == ip {
							table.Select(i+1, 0)
							break
						}
					}
				}
				pages.RemovePage("popup")
			}).SetAutocompleteFunc(func(currentText string) []string {
				if len(currentText) == 0 {
					return autoCompleteIPs
				}
				res := make([]string, 0, len(autoCompleteIPs))
				for _, v := range autoCompleteIPs {
					if strings.Contains(v, currentText) {
						res = append(res, v)
					}
				}
				return res
			}).SetFieldWidth(42)
			pages.AddPage("popup", popup, false, true)
		case tcell.KeyF2:
			popup := tview.NewInputField()
			popup.SetLabel("Search Host: ").SetDoneFunc(func(buttonIndex tcell.Key) {
				host := ""
				if buttonIndex == tcell.KeyEnter {
					host = popup.GetText()
				}
				if host != "" {
					for i, cfg := range configs {
						if cfg.Host == host {
							table.Select(i+1, 0)
							break
						}
					}
				}
				pages.RemovePage("popup")
			}).SetAutocompleteFunc(func(currentText string) []string {
				if len(currentText) == 0 {
					return autoCompleteHosts
				}
				res := make([]string, 0, len(autoCompleteHosts))
				for _, v := range autoCompleteHosts {
					if strings.Contains(v, currentText) {
						res = append(res, v)
					}
				}
				return res

			}).SetFieldWidth(15)
			pages.AddPage("popup", popup, false, true)
		case 's':
		}
		return event
	})

	pages.AddPage("main", table, true, true)

	if err := app.SetRoot(pages, true).Run(); err != nil {
		panic(err)
	}
}

func main() {
	configPath := os.Getenv("SSH_CONFIG")
	if configPath == "" {

		configPath = os.Getenv("HOME") + "/.ssh/config"
	}
	configs, err := ssh.ParseSSHConfig(configPath)
	if err != nil {
		fmt.Printf("Error parsing SSH config: %v\n", err)
		return
	}

	for _, cfg := range configs {
		autoCompleteHosts = append(autoCompleteHosts, cfg.Host)
		autoCompleteIPs = append(autoCompleteIPs, cfg.HostName)
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		fmt.Println("Cannot access config dir: ", err.Error())
		os.Exit(1)
	}
	masterKeyFile := filepath.Join(configDir, "master.key")
	credsFile := filepath.Join(configDir, "credentials.enc")

	var creds credentials.Credentials
	key, err := credentials.LoadMasterKey(masterKeyFile)
	if err == nil {
		creds, err := credentials.LoadCredentials(credsFile, key)
		if err != nil {
			fmt.Println("Error loading creds:", creds)
			os.Exit(1)
		}
	}

	displaySSHConfig(configs, creds)
}
