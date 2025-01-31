package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/rivo/tview"
)

var autoCompleteIPs []string
var autoCompleteHosts []string

func displaySSHConfig(configs []ssh.SSHConfig) {
	app := tview.NewApplication()

	pages := tview.NewPages()

	table := tview.NewTable().
		SetBorders(false)

	table.SetSelectable(true, false)

	table.SetCell(0, 0, tview.NewTableCell("Host").
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
	table.SetCell(0, 3, tview.NewTableCell("HostName").
		SetTextColor(tcell.ColorRed).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	table.SetCell(0, 4, tview.NewTableCell("IdentityFile").
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

	}

	table.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorViolet))
	table.SetSelectedFunc(func(row, column int) {
		selectedConfig := configs[row-1]
		popup := tview.NewModal().
			SetText(fmt.Sprintf("Host: %s\nUser: %s\nPort: %s\nHostName: %s\nIdentityFile: %s",
				selectedConfig.Host, selectedConfig.User, selectedConfig.Port, selectedConfig.HostName, selectedConfig.IdentityFile)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				pages.RemovePage("popup")
			})

		pages.AddPage("popup", popup, false, true)
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

	displaySSHConfig(configs)
}
