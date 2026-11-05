package tui

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/noboruma/s1h/internal/config"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/rivo/tview"
)

// ForwardStore persists the forwards added through the [p] popup. Forwards
// from ~/.ssh/config (LocalForward, set with [e]dit) are always on; these
// ones can be disabled.
type ForwardStore interface {
	Load() (map[string][]config.SavedForward, error)
	Save(map[string][]config.SavedForward) error
}

var SavedForwards ForwardStore

// savedForward is a forward [R]/[F] reopen when enabled.
type savedForward struct {
	spec       ssh.Forward
	fromConfig bool // LocalForward in ~/.ssh/config: always enabled
	enabled    bool
}

func (h *hostEntry) findSaved(spec ssh.Forward) (savedForward, bool) {
	for _, s := range h.saved {
		if s.spec == spec {
			return s, true
		}
	}
	return savedForward{}, false
}

// openForward returns the running forward for spec, if any.
func (h *hostEntry) openForward(spec ssh.Forward) *ssh.PortForward {
	for _, f := range h.forwards {
		if f.Spec == spec {
			return f
		}
	}
	return nil
}

// forwardSpecs lists saved forwards first, then open ones that are not saved.
func (h *hostEntry) forwardSpecs() []ssh.Forward {
	var specs []ssh.Forward
	for _, s := range h.saved {
		specs = append(specs, s.spec)
	}
	for _, f := range h.forwards {
		if !slices.Contains(specs, f.Spec) {
			specs = append(specs, f.Spec)
		}
	}
	return specs
}

func (u *hostUI) loadForwards() error {
	u.fwdState = map[string][]config.SavedForward{}
	var err error
	if SavedForwards != nil {
		var st map[string][]config.SavedForward
		if st, err = SavedForwards.Load(); err == nil {
			u.fwdState = st
		}
	}
	for _, h := range u.table.Snapshot() {
		u.syncSaved(h)
	}
	return err
}

// syncSaved rebuilds h.saved from ~/.ssh/config and s1h's own forwards.
func (u *hostUI) syncSaved(h *hostEntry) {
	h.saved = h.saved[:0]
	for _, spec := range h.cfg.LocalForwards {
		if _, dup := h.findSaved(spec); !dup {
			h.saved = append(h.saved, savedForward{spec: spec, fromConfig: true, enabled: true})
		}
	}
	for _, a := range u.fwdState[h.cfg.Host] {
		if _, dup := h.findSaved(a.Forward); !dup {
			h.saved = append(h.saved, savedForward{spec: a.Forward, enabled: !a.Disabled})
		}
	}
}

// updateState applies fn to h's s1h forwards, then saves them.
func (u *hostUI) updateState(h *hostEntry, fn func([]config.SavedForward) []config.SavedForward) error {
	u.fwdState[h.cfg.Host] = fn(u.fwdState[h.cfg.Host])
	u.syncSaved(h)
	u.table.Render()
	u.refresh()
	if SavedForwards == nil {
		return fmt.Errorf("saving port forwards is not available")
	}
	return SavedForwards.Save(u.fwdState)
}

func errFromConfig(spec ssh.Forward) error {
	return fmt.Errorf("%s comes from ~/.ssh/config and is always on.\nChange it with %s.", spec, tview.Escape("[e]dit"))
}

// addForward saves spec (enabled) unless it already is.
func (u *hostUI) addForward(h *hostEntry, spec ssh.Forward) error {
	if s, ok := h.findSaved(spec); ok {
		if s.enabled {
			return nil
		}
		return u.setEnabled(h, spec, true)
	}
	return u.updateState(h, func(st []config.SavedForward) []config.SavedForward {
		return append(st, config.SavedForward{Forward: spec})
	})
}

func (u *hostUI) setEnabled(h *hostEntry, spec ssh.Forward, on bool) error {
	s, ok := h.findSaved(spec)
	if !ok {
		return u.addForward(h, spec)
	}
	if s.fromConfig {
		return errFromConfig(spec)
	}
	return u.updateState(h, func(st []config.SavedForward) []config.SavedForward {
		for i := range st {
			if st[i].Forward == spec {
				st[i].Disabled = !on
			}
		}
		return st
	})
}

// forgetForward closes spec if open and drops it from s1h's saved forwards.
func (u *hostUI) forgetForward(h *hostEntry, spec ssh.Forward) error {
	s, saved := h.findSaved(spec)
	if saved && s.fromConfig {
		return errFromConfig(spec)
	}
	if f := h.openForward(spec); f != nil {
		u.closeForward(h, f)
	}
	if !saved {
		return nil
	}
	return u.updateState(h, func(st []config.SavedForward) []config.SavedForward {
		return slices.DeleteFunc(st, func(a config.SavedForward) bool { return a.Forward == spec })
	})
}

// dropHost closes h's forwards and forgets its s1h forwards (host removed).
func (u *hostUI) dropHost(h *hostEntry) error {
	closeAllForwards([]*hostEntry{h})
	if len(u.fwdState[h.cfg.Host]) == 0 {
		return nil
	}
	delete(u.fwdState, h.cfg.Host)
	if SavedForwards == nil {
		return nil
	}
	return SavedForwards.Save(u.fwdState)
}

// forwardPopup lists the selected host's saved and open port forwards, or
// goes straight to the "new forward" form when there are none.
func (u *hostUI) forwardPopup(h *hostEntry) {
	if len(h.forwardSpecs()) == 0 {
		u.newForwardForm(h)
		return
	}
	u.showForwardList(h, 0)
}

func (u *hostUI) showForwardList(h *hostEntry, selected int) {
	specs := h.forwardSpecs()
	list := tview.NewList()
	list.AddItem("+ New port forward", "", 'n', func() {
		u.closePopup()
		u.newForwardForm(h)
	})
	for _, spec := range specs {
		main, secondary := forwardListText(h, spec)
		list.AddItem(main, secondary, 0, func() {
			if f := h.openForward(spec); f != nil {
				u.closeForward(h, f)
				u.refreshForwardList(h, list.GetCurrentItem())
				return
			}
			u.openForwards(map[*hostEntry][]ssh.Forward{h: {spec}}, func() {
				u.refreshForwardList(h, list.GetCurrentItem())
			})
		})
	}
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		i := list.GetCurrentItem()
		if i == 0 {
			return event
		}
		spec := specs[i-1]
		var err error
		switch {
		case event.Rune() == ' ':
			s, ok := h.findSaved(spec)
			err = u.setEnabled(h, spec, !ok || !s.enabled)
		case event.Rune() == 'x' || event.Key() == tcell.KeyDelete:
			err = u.forgetForward(h, spec)
		default:
			return event
		}
		u.refreshForwardList(h, i)
		if err != nil {
			infoPopup(u.pages, err.Error())
		}
		return nil
	})
	list.SetCurrentItem(min(selected, list.GetItemCount()-1))

	help := tview.NewTextView().SetDynamicColors(true).SetText(
		" [yellow::b]Enter[-::-] open/close  [yellow::b]Space[-::-] enable/disable for [R[]/[F[]  [yellow::b]x[-::-] forget")
	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(list, 0, 1, true).
		AddItem(help, 1, 0, false)
	box.SetBorder(true).SetTitle(fmt.Sprintf(" Port forwards on %s ", h.cfg.Host))
	u.pages.AddPage("popup", modal(box, 80, min(2*list.GetItemCount()+3, 23)), true, true)
}

func (u *hostUI) refreshForwardList(h *hostEntry, selected int) {
	u.closePopup()
	if len(h.forwardSpecs()) > 0 {
		u.showForwardList(h, selected)
	}
}

func forwardListText(h *hostEntry, spec ssh.Forward) (main, secondary string) {
	var state []string
	mark := "[gray]○[-]"
	if f := h.openForward(spec); f != nil {
		mark = "[green]●[-]"
		state = append(state, fmt.Sprintf("open, %d active connection(s)", f.ActiveConns()))
	} else {
		state = append(state, "closed")
	}
	s, saved := h.findSaved(spec)
	switch {
	case !saved:
		state = append(state, "not saved")
	case s.fromConfig:
		state = append(state, "always on (~/.ssh/config)")
	case s.enabled:
		state = append(state, "enabled")
	default:
		state = append(state, "[red]disabled[-]")
	}
	return mark + " " + tview.Escape(spec.String()), strings.Join(state, " · ")
}

func (u *hostUI) newForwardForm(h *hostEntry) {
	form := tview.NewForm()
	local := tview.NewInputField().SetLabel("Local port (or addr:port): ").SetFieldWidth(24)
	remote := tview.NewInputField().SetLabel("Remote (host:port, from host): ").SetFieldWidth(24)
	form.AddFormItem(local).AddFormItem(remote)
	form.AddButton("Open", func() {
		l, r := strings.TrimSpace(local.GetText()), strings.TrimSpace(remote.GetText())
		if r == "" {
			r = l
		}
		if l == "" {
			infoPopup(u.pages, "Local port is required")
			return
		}
		spec := ssh.NewForward(l, r)
		u.closePopup()
		saveErr := u.addForward(h, spec)
		u.openForwards(map[*hostEntry][]ssh.Forward{h: {spec}}, func() {
			if saveErr != nil {
				infoPopup(u.pages, fmt.Sprintf("Port forward is open but could not be saved:\n%v", saveErr))
			}
		})
	})
	form.AddButton("Cancel", u.closePopup)
	u.showForm(fmt.Sprintf("Port forward via %s", h.cfg.Host), form, 9)
}

// reloadForwards opens every enabled saved forward of hosts not already open.
func (u *hostUI) reloadForwards(hosts []*hostEntry) {
	targets := map[*hostEntry][]ssh.Forward{}
	for _, h := range hosts {
		for _, s := range h.saved {
			if s.enabled && h.openForward(s.spec) == nil {
				targets[h] = append(targets[h], s.spec)
			}
		}
	}
	if len(targets) == 0 {
		msg := "All enabled port forwards are already open"
		if len(hosts) == 1 && hosts[0].enabledCount() == 0 {
			msg = fmt.Sprintf("No enabled port forwards for %s", hosts[0].cfg.Host)
		} else if u.table.SavedForwardCount() == 0 {
			msg = "No enabled port forwards"
		}
		infoPopup(u.pages, msg)
		return
	}
	u.openForwards(targets, nil)
}

func (h *hostEntry) enabledCount() int {
	n := 0
	for _, s := range h.saved {
		if s.enabled {
			n++
		}
	}
	return n
}

type forwardResult struct {
	h    *hostEntry
	spec ssh.Forward
	f    *ssh.PortForward
	err  error
}

// openForwards starts the forwards in the background, since each one dials
// SSH, and shows a summary unless everything opened. then, if set, runs on
// the UI goroutine once all are done.
func (u *hostUI) openForwards(targets map[*hostEntry][]ssh.Forward, then func()) {
	var jobs []forwardResult
	for h, specs := range targets {
		for _, spec := range specs {
			key := h.cfg.Host + "|" + spec.Local
			if u.pendingForwards[key] {
				continue
			}
			u.pendingForwards[key] = true
			jobs = append(jobs, forwardResult{h: h, spec: spec})
		}
	}
	if len(jobs) == 0 {
		return
	}
	cfgs := make([]ssh.SSHConfig, len(jobs))
	for i, j := range jobs {
		cfgs[i] = j.h.cfg
	}

	progress := tview.NewModal().SetText(fmt.Sprintf("Opening %d port forward(s)…", len(jobs)))
	u.pages.AddPage("popup", progress, false, true)

	go func() {
		var wg sync.WaitGroup
		for i := range jobs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				jobs[i].f, jobs[i].err = ssh.StartPortForward(cfgs[i], jobs[i].spec)
			}()
		}
		wg.Wait()
		u.app.QueueUpdateDraw(func() {
			if u.pages.GetPage("popup") == progress {
				u.closePopup()
			}
			var failed []string
			for _, j := range jobs {
				delete(u.pendingForwards, j.h.cfg.Host+"|"+j.spec.Local)
				switch {
				case j.err != nil:
					failed = append(failed, fmt.Sprintf("%s %s: %v", j.h.cfg.Host, j.spec, j.err))
				case !slices.Contains(u.table.Snapshot(), j.h):
					j.f.Close() // host removed meanwhile
				default:
					j.h.forwards = append(j.h.forwards, j.f)
				}
			}
			u.table.Render()
			u.refresh()
			if then != nil {
				then()
			}
			if len(failed) > 0 {
				infoPopup(u.pages, fmt.Sprintf("Opened %d/%d port forward(s)\n\n%s",
					len(jobs)-len(failed), len(jobs), tview.Escape(strings.Join(failed, "\n"))))
			} else if then == nil {
				infoPopup(u.pages, fmt.Sprintf("Opened %d port forward(s)", len(jobs)))
			}
		})
		// Same full repaint the key handler does, so wide glyphs don't leave
		// stale cells behind.
		u.app.Sync()
	}()
}

func (u *hostUI) closeForward(h *hostEntry, f *ssh.PortForward) {
	f.Close()
	h.forwards = slices.DeleteFunc(h.forwards, func(v *ssh.PortForward) bool { return v == f })
	u.table.Render()
	u.refresh()
}

func closeAllForwards(hosts []*hostEntry) {
	for _, h := range hosts {
		for _, f := range h.forwards {
			f.Close()
		}
		h.forwards = nil
	}
}
