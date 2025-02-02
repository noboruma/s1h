package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/noboruma/s1h/internal/credentials"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/rivo/tview"
)

var (
	autoCompleteHostNames []string
	autoCompleteHosts     []string
)

const (
	masterKeyFileName = "master.key"
	credsFileName     = "credentials.enc"
)

func displaySSHConfig(configs []ssh.SSHConfig) {
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
	table.SetCell(0, 1, tview.NewTableCell("User").
		SetTextColor(tcell.ColorBlueViolet).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	table.SetCell(0, 2, tview.NewTableCell("Port").
		SetTextColor(tcell.ColorGreen).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	table.SetCell(0, 3, tview.NewTableCell("HostName (F4)").
		SetTextColor(tcell.ColorRed).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	table.SetCell(0, 4, tview.NewTableCell("Auth").
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
		if config.Password != "" && config.IdentityFile == "" {
			table.SetCell(i+1, 4, tview.NewTableCell("Password").
				SetAlign(tview.AlignLeft))
		} else if config.IdentityFile != "" {
			table.SetCell(i+1, 4, tview.NewTableCell("Key").
				SetAlign(tview.AlignLeft))
		}
	}

	go reachabilityCheck(configs, table, app)

	//table.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorViolet))
	table.SetSelectedFunc(func(row, column int) {
		selectedConfig := configs[row-1]
		if selectedConfig.Password == "" && selectedConfig.IdentityFile == "" {
			infoPopup(pages, fmt.Sprintf("Missing credentials for Host: %s\nUser: %s\nPort: %s\nHostName: %s\nIdentityFile: %s",
				selectedConfig.Host,
				selectedConfig.User,
				selectedConfig.Port,
				selectedConfig.HostName,
				selectedConfig.IdentityFile))
			return
		}

		pages.AddPage("popup", sshPage, true, true)
		app.Suspend(func() {
			err := ssh.ExecuteSSHShell(selectedConfig)
			pages.RemovePage("popup")
			if err != nil {
				infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
					selectedConfig.Host, err))
			}
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
		case tcell.KeyF4:
			searchFilterPopup("HostName", pages, table, configs,
				func(cfg ssh.SSHConfig, match string) bool {
					return cfg.HostName == match
				}, autoCompleteHostNames)
		case tcell.KeyF1:
			searchFilterPopup("Host", pages, table, configs,
				func(cfg ssh.SSHConfig, match string) bool {
					return cfg.Host == match
				}, autoCompleteHosts)
		}
		switch event.Rune() {
		case 'q':
			if pages.HasPage("popup") {
				pages.RemovePage("popup")
			} else {
				app.Stop()
			}
		case 'c': // copy to
			row, _ := table.GetSelection()
			selectedConfig := configs[row-1]
			client, err := ssh.SSHClient(selectedConfig)
			if err != nil {
				infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
					selectedConfig.Host, err))
				return event
			}
			popup := tview.NewForm()
			fromField := tview.NewInputField().SetFieldWidth(256)
			fromField.SetLabel("From (local): ").
				SetAutocompleteFunc(DirAutocomplete)
			toField := tview.NewInputField().SetFieldWidth(256)
			toField.SetLabel("To (remote): ")

			popup.AddFormItem(fromField)
			popup.AddFormItem(toField)
			popup.AddButton("Upload", func() {
				err := ssh.UploadFile(client, fromField.GetText(), toField.GetText())
				pages.RemovePage("popup")
				if err != nil {
					infoPopup(pages,
						fmt.Sprintf("Error uploading %s -> %s: %v",
							fromField.GetText(),
							toField.GetText(), err))
				} else {
					infoPopup(pages, "Successfully uploaded")
				}
			})
			popup.SetCancelFunc(func() {
				pages.RemovePage("popup")
			})
			pages.AddPage("popup", popup, true, true)
		case 'C': // copy from
			row, _ := table.GetSelection()
			selectedConfig := configs[row-1]
			client, err := ssh.SSHClient(selectedConfig)
			if err != nil {
				infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
					selectedConfig.Host, err))
				return event
			}
			popup := tview.NewForm()
			fromField := tview.NewInputField()
			fromField.SetLabel("From (remote): ").SetFieldWidth(256)

			toField := tview.NewInputField().SetFieldWidth(256)
			toField.SetLabel("To (local): ").
				SetAutocompleteFunc(DirAutocomplete)

			popup.AddFormItem(fromField)
			popup.AddFormItem(toField)
			popup.AddButton("Download", func() {
				err := ssh.UploadFile(client, fromField.GetText(), toField.GetText())
				pages.RemovePage("popup")
				if err != nil {
					infoPopup(pages,
						fmt.Sprintf("Error downloading %s -> %s: %v",
							fromField.GetText(),
							toField.GetText(), err))
				} else {
					infoPopup(pages, "Successfully downloaded")
				}
			})
			popup.SetCancelFunc(func() {
				pages.RemovePage("popup")
			})
			pages.AddPage("popup", popup, true, true)
		}
		return event
	})

	pages.AddPage("main", table, true, true)

	if err := app.SetRoot(pages, true).Run(); err != nil {
		panic(err)
	}
}

func reachabilityCheck(configs []ssh.SSHConfig, table *tview.Table, app *tview.Application) {
	for ; ; <-time.After(2 * time.Minute) {
		for i, config := range configs {
			port, err := strconv.Atoi(config.Port)
			if err != nil {
				port = 22
			}
			go func(i int) {
				if ssh.CheckSSHPort(config.HostName, port, 10*time.Second) {
					table.GetCell(i+1, 0).SetTextColor(tcell.ColorDarkGreen)
				} else {
					table.GetCell(i+1, 0).SetTextColor(tcell.ColorDarkRed)
				}
				app.Draw()
			}(i)
		}
	}
}

func DirAutocomplete(currentText string) []string {
	var dir string
	if filepath.IsAbs(currentText) {
		dir = filepath.Dir(currentText)
	} else {
		dir, _ = os.Getwd()
		currentText = filepath.Join(dir, currentText)
	}
	files, err := os.ReadDir(dir)
	var res []string
	if err != nil {
		return res
	}

	info, err := os.Stat(currentText)
	if err != nil || !info.IsDir() {
		for _, file := range files {
			fullpath := filepath.Join(dir, file.Name())
			fullpath = filepath.Clean(fullpath)
			if strings.HasPrefix(fullpath, currentText) {
				res = append(res, fullpath)
			}
		}
	} else {
		res = make([]string, 0, len(files))
		for _, file := range files {
			fullpath := filepath.Join(dir, file.Name())
			res = append(res, fullpath)
		}
	}
	return res
}

func infoPopup(pages *tview.Pages, msg string) {
	popup := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("popup")
		})
	pages.AddPage("popup", popup, false, true)
}

func feedsCredentialsToConfig(creds credentials.Credentials, configs []ssh.SSHConfig) {
	for i, cfg := range configs {
		cfg.Password = creds.Entries[cfg.Host]
		configs[i] = cfg
	}
}

func searchFilterPopup(fieldName string, pages *tview.Pages, table *tview.Table,
	configs []ssh.SSHConfig,
	match func(cfg ssh.SSHConfig, inputText string) bool,
	autoCompleteEntries []string) {
	popup := tview.NewInputField()
	popup.SetLabel(fmt.Sprintf("Search %s: ", fieldName)).SetDoneFunc(func(buttonIndex tcell.Key) {
		hostname := ""
		if buttonIndex == tcell.KeyEnter {
			hostname = popup.GetText()
		}
		if hostname != "" {
			for i, cfg := range configs {
				if match(cfg, hostname) {
					table.Select(i+1, 0)
					break
				}
			}
		}
		pages.RemovePage("popup")
	}).SetAutocompleteFunc(func(currentText string) []string {
		if len(currentText) == 0 {
			return autoCompleteEntries
		}
		res := make([]string, 0, len(autoCompleteEntries))
		for _, v := range autoCompleteEntries {
			if strings.Contains(v, currentText) {
				res = append(res, v)
			}
		}
		return res
	}).SetFieldWidth(42)
	pages.AddPage("popup", popup, false, true)
}

func main() {
	configPath := os.Getenv("SSH_CONFIG")
	if configPath == "" {
		configPath = os.Getenv("HOME") + "/.ssh/config"
	}

	configs, err := ssh.ParseSSHConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading creds: %v\n", err)
	}

	for _, cfg := range configs {
		autoCompleteHosts = append(autoCompleteHosts, cfg.Host)
		autoCompleteHostNames = append(autoCompleteHostNames, cfg.HostName)
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("Error loading creds: %v\n", err)
	}
	masterKeyFile := filepath.Join(configDir, masterKeyFileName)
	credsFile := filepath.Join(configDir, credsFileName)

	var creds credentials.Credentials
	key, err := credentials.LoadMasterKey(masterKeyFile)
	if err == nil {
		creds, err = credentials.LoadCredentials(credsFile, key)
		if err != nil {
			log.Fatalf("Error loading creds: %v\n", err)
		}
		feedsCredentialsToConfig(creds, configs)
	}

	displaySSHConfig(configs)
}
