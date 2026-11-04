package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/rivo/tview"
)

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

	table := newHostTable(configs)

	info := tview.NewTextView().SetDynamicColors(true)
	refreshInfo := func() {
		info.SetText(headerInfo(len(table.visible), len(table.hosts),
			table.ReachableCount(), len(table.Marked())))
	}
	refreshInfo()

	logo := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignRight).
		SetText(logoText())

	header := tview.NewFlex().
		AddItem(info, 0, 1, false).
		AddItem(logo, logoWidth+1, 0, false)

	filterInput := tview.NewInputField().
		SetLabel(" / ").
		SetLabelColor(tcell.ColorYellow).
		SetFieldBackgroundColor(tcell.ColorDefault)
	filterRow := tview.NewFlex().SetDirection(tview.FlexRow)

	root.AddItem(header, len(logoLines), 0, false)
	root.AddItem(filterRow, 0, 0, false)
	root.AddItem(pages, 0, 1, true)

	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	footer.SetBackgroundColor(footerBg)
	root.AddItem(footer, 1, 0, false)
	footerWidth := -1
	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		if w, _ := screen.Size(); w != footerWidth {
			footerWidth = w
			text, lines := footerText(w)
			footer.SetText(text)
			root.ResizeItem(footer, lines, 0)
		}
		return false
	})

	showFilter := func(show bool) {
		filterRow.Clear()
		if show {
			filterRow.AddItem(filterInput, 1, 0, false)
			root.ResizeItem(filterRow, 1, 0)
		} else {
			root.ResizeItem(filterRow, 0, 0)
		}
	}
	applyFilter := func(text string) {
		table.SetFilter(text)
		refreshInfo()
	}
	filterInput.SetChangedFunc(applyFilter)
	filterInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			filterInput.SetText("")
		}
		showFilter(filterInput.GetText() != "")
		app.SetFocus(table)
	})

	check := func(h *hostEntry) { go checkHost(h, app, table, refreshInfo) }
	go reachabilityCheck(table, check)
	go spinStatus(table, app)

	ui := &hostUI{app: app, pages: pages, table: table, refresh: refreshInfo, check: check}

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if app.GetFocus() == filterInput {
			return event
		}
		defer app.Sync()
		switch event.Key() {
		case tcell.KeyEscape:
			if pages.HasPage("popup") {
				pages.RemovePage("popup")
			} else if table.filter != "" {
				filterInput.SetText("")
				showFilter(false)
			} else {
				app.Stop()
			}
			return nil
		}
		if pages.HasPage("popup") {
			return event
		}
		h, ok := table.Selected()
		marked := table.Marked()
		switch event.Rune() {
		case 'q':
			app.Stop()
		case 's':
			if !ok {
				return nil
			}
			selectedConfig := h.cfg
			pages.AddPage("popup", sshPage, true, true)
			app.Suspend(func() {
				appSuspended.Store(true)
				defer appSuspended.Store(false)
				err := ssh.ExecuteSSHShell(selectedConfig)
				pages.RemovePage("popup")
				if err != nil {
					infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
						selectedConfig.Host, err))
				}
			})
			return nil
		case 'u':
			if len(marked) != 0 {
				multiCopyTo(pages, marked)
			} else if ok {
				singleCopyTo(pages, h.cfg)
			}
			return nil
		case 'd':
			if len(marked) != 0 {
				multiCopyFrom(pages, marked)
			} else if ok {
				singleCopyFrom(pages, h.cfg)
			}
			return nil
		case 'x':
			if len(marked) != 0 {
				multiExecOn(pages, marked)
			} else if ok {
				singleExecOn(pages, h.cfg)
			}
			return nil
		case 'm':
			if !ok {
				return nil
			}
			if !h.marked && !h.reachable.Load() {
				infoPopup(pages, fmt.Sprintf("Host %s: is not reachable", h.cfg.Host))
				return nil
			}
			h.marked = !h.marked
			table.Render()
			refreshInfo()
			return nil
		case 'M':
			if len(marked) == 0 {
				for _, v := range table.visible {
					v.marked = v.reachable.Load()
				}
			} else {
				for _, v := range table.hosts {
					v.marked = false
				}
			}
			table.Render()
			refreshInfo()
			return nil
		case 'e':
			if ok {
				ui.editPopup(h)
			}
			return nil
		case '+':
			ui.addPopup()
			return nil
		case '-':
			if ok {
				ui.removePopup(h)
			}
			return nil
		case 'r':
			if !ok {
				return nil
			}
			if h.cfg.Password == "" {
				infoPopup(pages, fmt.Sprintf("No stored password for %s", h.cfg.Host))
			} else {
				infoPopup(pages, fmt.Sprintf("Password for %s:\n\n%s", h.cfg.Host, tview.Escape(h.cfg.Password)))
			}
			return nil
		case '/', '?':
			showFilter(true)
			app.SetFocus(filterInput)
			return nil
		}
		return event
	})

	pages.AddPage("main", table, true, true)
	if StartupWarning != "" {
		infoPopup(pages, StartupWarning)
	}

	if err := app.SetRoot(root, true).SetFocus(table).Run(); err != nil {
		panic(err)
	}
}

var appSuspended atomic.Bool

func reachabilityCheck(table *hostTable, check func(*hostEntry)) {
	for ; ; <-time.After(2 * time.Minute) {
		for _, h := range table.Snapshot() {
			check(h)
		}
	}
}

func checkHost(h *hostEntry, app *tview.Application, table *hostTable, onUpdate func()) {
	var hostname, portStr string
	done := make(chan struct{})
	app.QueueUpdate(func() {
		hostname, portStr = h.cfg.HostName, h.cfg.Port
		close(done)
	})
	<-done
	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = 22
	}
	ok := ssh.CheckSSHPort(hostname, port, 10*time.Second)
	h.reachable.Store(ok)
	h.checked.Store(true)
	app.QueueUpdateDraw(func() {
		if !ok {
			h.marked = false
		}
		table.Render()
		onUpdate()
	})
}

func spinStatus(table *hostTable, app *tview.Application) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if appSuspended.Load() || !table.Pending() {
			continue
		}
		app.QueueUpdateDraw(func() {
			table.frame++
			table.Render()
		})
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

func modal(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
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
		err := ssh.UploadFile(client, fromField.GetText(), toField.GetText(), nil)
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
	clients, err := ssh.InitMultiClients(selectedConfigs)
	if err != nil {
		infoPopup(pages, err.Error())
		return
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
			err := ssh.UploadFile(clients[i], fromField.GetText(), toField.GetText(), nil)
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
	clients, err := ssh.InitMultiClients(selectedConfigs)
	if err != nil {
		infoPopup(pages, err.Error())
		return
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
		err := ssh.DownloadFile(client, fromField.GetText(), toField.GetText(), nil)
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
	clients, err := ssh.InitMultiClients(selectedConfigs)
	if err != nil {
		infoPopup(pages, err.Error())
		return
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
		if !strings.Contains(toField.GetText(), "*") {
			infoPopup(pages, "Please specify a * in the 'To:' to differentiate the downloaded files\nfor instance: /tmp/toto_*.tar.gz or /tmp/*/toto.tar.gz")
			return
		}
		successCount := 0
		for i := range clients {
			ssh.PutSCPDownloadEntry(selectedConfigs[i].Host, ssh.SCPHistoryEntry{
				From: fromField.GetText(),
				To:   toField.GetText(),
			})
			toPath := strings.ReplaceAll(toField.GetText(), "*", selectedConfigs[i].Host)
			err := ssh.DownloadFile(clients[i], fromField.GetText(), toPath, nil)
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
