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
	cssh "golang.org/x/crypto/ssh"
)

var (
	autoCompleteHostNames []string
	autoCompleteHosts     []string
	multiSelectConfigs    []ssh.SSHConfig
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
	header.SetCell(1, 1, tview.NewTableCell("Copy local file to selected remote(s)").
		SetTextColor(tcell.ColorPurple).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(2, 0, tview.NewTableCell("<shift>+c:").
		SetTextColor(tcell.ColorYellow).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(2, 1, tview.NewTableCell("Copy from selected remote to local file(s)").
		SetTextColor(tcell.ColorPurple).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(3, 0, tview.NewTableCell("Enter:").
		SetTextColor(tcell.ColorYellow).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(3, 1, tview.NewTableCell("Shell to selected host").
		SetTextColor(tcell.ColorPurple).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(4, 0, tview.NewTableCell("m:").
		SetTextColor(tcell.ColorBlue).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(4, 1, tview.NewTableCell("Multi select host (toggle)").
		SetTextColor(tcell.ColorPurple).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(5, 0, tview.NewTableCell("M:").
		SetTextColor(tcell.ColorBlue).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(5, 1, tview.NewTableCell("Clear all multi selected hosts").
		SetTextColor(tcell.ColorPurple).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(6, 0, tview.NewTableCell("e:").
		SetTextColor(tcell.ColorYellow).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	header.SetCell(6, 1, tview.NewTableCell("Execute short-lived command").
		SetTextColor(tcell.ColorPurple).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))

	root.AddItem(header, 7, 2, false)

	tableHeader := tview.NewTable().
		SetSeparator('|').
		SetBorders(false).
		SetSelectable(false, false)

	tableHeader.SetCell(0, 0, tview.NewTableCell("Host (F1)").
		SetTextColor(tcell.ColorBlue).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	tableHeader.SetCell(0, 1, tview.NewTableCell("HostName (F2)").
		SetTextColor(tcell.ColorRed).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	tableHeader.SetCell(0, 2, tview.NewTableCell("Port").
		SetTextColor(tcell.ColorGreen).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	tableHeader.SetCell(0, 3, tview.NewTableCell("User").
		SetTextColor(tcell.ColorBlueViolet).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))
	tableHeader.SetCell(0, 4, tview.NewTableCell("Auth").
		SetTextColor(tcell.ColorBlue).
		SetAlign(tview.AlignLeft).
		SetSelectable(false))

	root.AddItem(tableHeader, 1, 5, false)

	root.AddItem(pages, 0, 15, true)

	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false)

	for i, config := range configs {
		table.SetCell(i, 0, tview.NewTableCell(config.Host).
			SetAlign(tview.AlignLeft))
		table.SetCell(i, 1, tview.NewTableCell(config.HostName).
			SetAlign(tview.AlignLeft))
		table.SetCell(i, 2, tview.NewTableCell(config.Port).
			SetAlign(tview.AlignLeft))
		table.SetCell(i, 3, tview.NewTableCell(config.User).
			SetAlign(tview.AlignLeft))
		if config.Password != "" && config.IdentityFile == "" {
			table.SetCell(i, 4, tview.NewTableCell("Pass").
				SetAlign(tview.AlignLeft))
		} else if config.IdentityFile != "" {
			table.SetCell(i, 4, tview.NewTableCell("Key").
				SetAlign(tview.AlignLeft))
		} else {
			table.SetCell(i, 4, tview.NewTableCell("All").
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
		defer app.Sync()
		switch event.Key() {
		case tcell.KeyEscape:
			if pages.HasPage("popup") {
				pages.RemovePage("popup")
			} else {
				app.Stop()
			}
		case tcell.KeyF2:
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
			if pages.HasPage("popup") {
				return event
			}
			if len(multiSelectConfigs) != 0 {
				multiCopyTo(pages, multiSelectConfigs)
			} else {
				row, _ := table.GetSelection()
				selectedConfig := configs[row]
				singleCopyTo(pages, selectedConfig)
			}
			return nil
		case 'C': // copy from
			if pages.HasPage("popup") {
				return event
			}
			if len(multiSelectConfigs) != 0 {
				multiCopyFrom(pages, multiSelectConfigs)
			} else {
				row, _ := table.GetSelection()
				selectedConfig := configs[row]
				singleCopyFrom(pages, selectedConfig)
			}
			return nil
		case 'm':
			if pages.HasPage("popup") {
				return event
			}
			row, _ := table.GetSelection()
			selectedConfig := configs[row]

			found := false
			for i := range multiSelectConfigs {
				if multiSelectConfigs[i].Host == selectedConfig.Host {
					multiSelectConfigs[i] = multiSelectConfigs[len(multiSelectConfigs)-1]
					multiSelectConfigs = multiSelectConfigs[:len(multiSelectConfigs)-1]
					table.GetCell(row, 0).SetBackgroundColor(tcell.ColorNone)
					found = true
					break
				}
			}
			if !found {
				multiSelectConfigs = append(multiSelectConfigs, selectedConfig)
				table.GetCell(row, 0).SetBackgroundColor(tcell.ColorBlue)
			}
		case 'M':
			if pages.HasPage("popup") {
				return event
			}
			for i := range multiSelectConfigs {
				for row := 1; row < table.GetRowCount(); row++ {
					selectedConfig := configs[row-1]
					if multiSelectConfigs[i].Host == selectedConfig.Host {
						table.GetCell(row, 0).SetBackgroundColor(tcell.ColorNone)
						break
					}
				}
			}
			multiSelectConfigs = multiSelectConfigs[:0]
			return nil
		case 'e':
			if pages.HasPage("popup") {
				return event
			}
			if len(multiSelectConfigs) != 0 {
				multiExecOn(pages, multiSelectConfigs)
			} else {
				row, _ := table.GetSelection()
				selectedConfig := configs[row]
				singleExecOn(pages, selectedConfig)
			}
			return nil
		case '/':
			fallthrough
		case '?':
			searchFilterPopup("Host", pages, table, configs,
				func(cfg ssh.SSHConfig, match string) bool {
					return cfg.Host == match
				}, autoCompleteHosts)
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

func singleCopyTo(pages *tview.Pages, selectedConfig ssh.SSHConfig) {
	client, err := ssh.SSHClient(selectedConfig)
	if err != nil {
		infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
			selectedConfig.Host, err))
		return
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
}

func multiCopyTo(pages *tview.Pages, selectedConfigs []ssh.SSHConfig) {
	var clients []*cssh.Client
	for i := range selectedConfigs {
		client, err := ssh.SSHClient(selectedConfigs[i])
		if err != nil {
			infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
				selectedConfigs[i].Host, err))
			return
		}
		clients = append(clients, client)
	}

	prevValues := ssh.GetSCPUploadEntry(selectedConfigs[0].Host)
	popup := tview.NewForm()
	fromField := tview.NewInputField().SetFieldWidth(256).SetText(prevValues.From)
	fromField.SetLabel("From (local): ").
		SetAutocompleteFunc(DirAutocomplete)
	toField := tview.NewInputField().SetFieldWidth(256).SetText(prevValues.To)
	toField.SetLabel("To (multiple remotes): ")

	popup.AddFormItem(fromField)
	popup.AddFormItem(toField)

	popup.AddButton("Upload", func() {
		successCount := 0
		for i := range clients {
			ssh.PutSCPUploadEntry(selectedConfigs[i].Host, ssh.SCPHistoryEntry{
				From: fromField.GetText(),
				To:   toField.GetText(),
			})
			err := ssh.UploadFile(clients[i], fromField.GetText(), toField.GetText())
			if err != nil {
				infoPopup(pages,
					fmt.Sprintf("Error uploading %s -> %s: %v",
						fromField.GetText(),
						toField.GetText(), err))
			} else {
				successCount++
			}
		}
		pages.RemovePage("popup")
		infoPopup(pages, fmt.Sprintf("Successfully uploaded: %d/%d", successCount, len(clients)))
	})
	popup.SetCancelFunc(func() {
		pages.RemovePage("popup")
	})
	pages.AddPage("popup", popup, true, true)
}

func singleExecOn(pages *tview.Pages, selectedConfig ssh.SSHConfig) {
	client, err := ssh.SSHClient(selectedConfig)
	if err != nil {
		infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
			selectedConfig.Host, err))
		return
	}
	prevValues := ssh.GetExecEntry(selectedConfig.Host)
	popup := tview.NewForm()
	cmdField := tview.NewInputField().SetFieldWidth(256).SetText(prevValues.Command)
	cmdField.SetLabel("Command: ").
		SetAutocompleteFunc(DirAutocomplete)
	popup.AddFormItem(cmdField)
	popup.AddButton("Execute", func() {
		ssh.PutExecEntry(selectedConfig.Host, ssh.ExecHistoryEntry{
			Command: cmdField.GetText(),
		})
		b, err := ssh.ExecCommand(client, cmdField.GetText())
		pages.RemovePage("popup")
		if err != nil {
			infoPopup(pages,
				fmt.Sprintf("Error executing %s on %s: %v",
					cmdField.GetText(),
					selectedConfig.Host,
					err))
		} else {
			infoPopup(pages, fmt.Sprintf("Successfully executed:\n%s\n", string(b)))
		}
	})
	popup.SetCancelFunc(func() {
		pages.RemovePage("popup")
	})
	pages.AddPage("popup", popup, true, true)
}

func multiExecOn(pages *tview.Pages, selectedConfigs []ssh.SSHConfig) {
	var clients []*cssh.Client
	for i := range selectedConfigs {
		client, err := ssh.SSHClient(selectedConfigs[i])
		if err != nil {
			infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
				selectedConfigs[i].Host, err))
			return
		}
		clients = append(clients, client)
	}

	prevValues := ssh.GetExecEntry(selectedConfigs[0].Host)
	popup := tview.NewForm()
	cmdField := tview.NewInputField().SetFieldWidth(256).SetText(prevValues.Command)
	cmdField.SetLabel("Command: ").
		SetAutocompleteFunc(DirAutocomplete)
	popup.AddFormItem(cmdField)

	successCount := 0
	popup.AddButton("Execute on all", func() {
		for i := range clients {
			ssh.PutExecEntry(selectedConfigs[i].Host, ssh.ExecHistoryEntry{
				Command: cmdField.GetText(),
			})
			_, err := ssh.ExecCommand(clients[i], cmdField.GetText())
			if err != nil {
				infoPopup(pages,
					fmt.Sprintf("Error executing %s on %s: %v",
						cmdField.GetText(),
						selectedConfigs[i].Host,
						err))
			} else {
				successCount++
			}
		}
		pages.RemovePage("popup")
		infoPopup(pages, fmt.Sprintf("Successfully executed: %d/%d", successCount, len(clients)))
	})
	popup.SetCancelFunc(func() {
		pages.RemovePage("popup")
	})
	pages.AddPage("popup", popup, true, true)
}

func singleCopyFrom(pages *tview.Pages, selectedConfig ssh.SSHConfig) {
	client, err := ssh.SSHClient(selectedConfig)
	if err != nil {
		infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
			selectedConfig.Host, err))
		return
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
		err := ssh.DownloadFile(client, fromField.GetText(), toField.GetText())
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

func multiCopyFrom(pages *tview.Pages, selectedConfigs []ssh.SSHConfig) {
	var clients []*cssh.Client
	for i := range selectedConfigs {
		client, err := ssh.SSHClient(selectedConfigs[i])
		if err != nil {
			infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
				selectedConfigs[i].Host, err))
			return
		}
		clients = append(clients, client)
	}

	prevValues := ssh.GetSCPDownloadEntry(selectedConfigs[0].Host)
	popup := tview.NewForm()
	fromField := tview.NewInputField()
	fromField.SetLabel("From (remote): ").SetFieldWidth(256).SetText(prevValues.From)

	toField := tview.NewInputField().SetFieldWidth(256).SetText(prevValues.To)
	toField.SetLabel("To (local, use * for hosts): ").
		SetAutocompleteFunc(DirAutocomplete)

	popup.AddFormItem(fromField)
	popup.AddFormItem(toField)
	popup.AddButton("Download", func() {
		if strings.Index(toField.GetText(), "*") == -1 {
			infoPopup(pages, "Please specify a * in the 'To:' to differenciate the downloaded files\nfor instance: /tmp/toto_*.tar.gz or /tmp/*/toto.tar.gz")
			return
		}
		successCount := 0
		for i := range clients {
			ssh.PutSCPDownloadEntry(selectedConfigs[i].Host, ssh.SCPHistoryEntry{
				From: fromField.GetText(),
				To:   toField.GetText(),
			})
			toPath := strings.ReplaceAll(toField.GetText(), "*", selectedConfigs[i].Host)
			err := ssh.DownloadFile(clients[i], fromField.GetText(), toPath)
			if err != nil {
				infoPopup(pages,
					fmt.Sprintf("Error downloading %s -> %s: %v",
						fromField.GetText(),
						toPath, err))
			} else {
				successCount++
			}
		}
		pages.RemovePage("popup")
		infoPopup(pages, fmt.Sprintf("Successfully downloaded: %d/%d", successCount, len(clients)))
	})
	popup.SetCancelFunc(func() {
		pages.RemovePage("popup")
	})
	pages.AddPage("popup", popup, true, true)
}
