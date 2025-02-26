package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/rivo/tview"
)

var (
	autoCompleteHostNames []string
	autoCompleteHosts     []string
)

func PopulateAutocompleteCaches(configs []ssh.SSHConfig) {
	for _, cfg := range configs {
		autoCompleteHosts = append(autoCompleteHosts, cfg.Host)
		autoCompleteHostNames = append(autoCompleteHostNames, cfg.HostName)
	}
}

func DisplaySSHConfig(configs []ssh.SSHConfig) {
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
	root := tview.NewFlex().SetDirection(tview.FlexRow)

	header := tview.NewTable()
	header.SetCell(0, 0, tview.NewTableCell("Commands").
		SetTextColor(tcell.ColorYellow).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(1, 0, tview.NewTableCell("c:").
		SetTextColor(tcell.ColorYellow).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(1, 1, tview.NewTableCell("Copy local file to selected remote").
		SetTextColor(tcell.ColorPurple).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(2, 0, tview.NewTableCell("<shift>+c:").
		SetTextColor(tcell.ColorYellow).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(2, 1, tview.NewTableCell("Copy from selected remote file to local").
		SetTextColor(tcell.ColorPurple).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(3, 0, tview.NewTableCell("Enter:").
		SetTextColor(tcell.ColorYellow).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(3, 1, tview.NewTableCell("SSH to selected host").
		SetTextColor(tcell.ColorPurple).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))

	tableHeader := tview.NewTable()
	tableHeader.SetSelectable(false, false)

	tableHeader.SetCell(0, 0, tview.NewTableCell("Host (F1)").
		SetTextColor(tcell.ColorBlue).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	tableHeader.SetCell(0, 1, tview.NewTableCell("User").
		SetTextColor(tcell.ColorBlueViolet).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	tableHeader.SetCell(0, 2, tview.NewTableCell("Port").
		SetTextColor(tcell.ColorGreen).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	tableHeader.SetCell(0, 3, tview.NewTableCell("HostName (F4)").
		SetTextColor(tcell.ColorRed).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	tableHeader.SetCell(0, 4, tview.NewTableCell("Auth").
		SetTextColor(tcell.ColorBlue).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))

	root.AddItem(header, 4, 1, true)
	root.AddItem(tableHeader, 1, 1, true)
	root.AddItem(pages, 0, 15, true)

	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false)

	for i, config := range configs {
		table.SetCell(i, 0, tview.NewTableCell(config.Host).
			SetAlign(tview.AlignLeft))
		table.SetCell(i, 1, tview.NewTableCell(config.User).
			SetAlign(tview.AlignLeft))
		table.SetCell(i, 2, tview.NewTableCell(config.Port).
			SetAlign(tview.AlignLeft))
		table.SetCell(i, 3, tview.NewTableCell(config.HostName).
			SetAlign(tview.AlignLeft))
		if config.Password != "" && config.IdentityFile == "" {
			table.SetCell(i, 4, tview.NewTableCell("Password").
				SetAlign(tview.AlignLeft))
		} else if config.IdentityFile != "" {
			table.SetCell(i, 4, tview.NewTableCell("Key").
				SetAlign(tview.AlignLeft))
		}
	}

	go reachabilityCheck(configs, table, app)

	table.SetSelectedFunc(func(row, column int) {
		selectedConfig := configs[row]

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
			if !pages.HasPage("popup") {
				app.Stop()
			}
		case 'c': // copy to
			if pages.HasPage("popup") {
				return event
			}
			row, _ := table.GetSelection()
			selectedConfig := configs[row]
			client, err := ssh.SSHClient(selectedConfig)
			if err != nil {
				infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
					selectedConfig.Host, err))
				return nil
			}
			prevValues := ssh.GetSCPUploadEntry(selectedConfig.Host)
			popup := tview.NewForm()
			fromField := tview.NewInputField().SetFieldWidth(256).SetText(prevValues.From)
			fromField.SetLabel("From (local): ").
				SetAutocompleteFunc(DirAutocomplete)
			toField := tview.NewInputField().SetFieldWidth(256).SetText(prevValues.To)
			toField.SetLabel("To (remote): ")

			popup.AddFormItem(fromField)
			popup.AddFormItem(toField)
			popup.AddButton("Upload", func() {
				ssh.PutSCPUploadEntry(selectedConfig.Host, ssh.SCPHistoryEntry{
					From: fromField.GetText(),
					To:   toField.GetText(),
				})
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
			return nil
		case 'C': // copy from
			if pages.HasPage("popup") {
				return event
			}
			row, _ := table.GetSelection()
			selectedConfig := configs[row]
			client, err := ssh.SSHClient(selectedConfig)
			if err != nil {
				infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
					selectedConfig.Host, err))
				return nil
			}
			prevValues := ssh.GetSCPDownloadEntry(selectedConfig.Host)
			popup := tview.NewForm()
			fromField := tview.NewInputField()
			fromField.SetLabel("From (remote): ").SetFieldWidth(256).SetText(prevValues.From)

			toField := tview.NewInputField().SetFieldWidth(256).SetText(prevValues.To)
			toField.SetLabel("To (local): ").
				SetAutocompleteFunc(DirAutocomplete)

			popup.AddFormItem(fromField)
			popup.AddFormItem(toField)
			popup.AddButton("Download", func() {
				ssh.PutSCPDownloadEntry(selectedConfig.Host, ssh.SCPHistoryEntry{
					From: fromField.GetText(),
					To:   toField.GetText(),
				})
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
			return nil
		}
		return event
	})

	pages.AddPage("main", table, true, true)

	if err := app.SetRoot(root, true).SetFocus(pages).Run(); err != nil {
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
					table.GetCell(i, 0).SetTextColor(tcell.ColorDarkGreen)
				} else {
					table.GetCell(i, 0).SetTextColor(tcell.ColorDarkRed)
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
					table.Select(i, 0)
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
