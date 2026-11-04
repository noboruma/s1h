package tui

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gdamore/tcell/v2"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/rivo/tview"
)

const (
	colStatus = 0
	colHost   = 1
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type hostEntry struct {
	cfg       ssh.SSHConfig
	marked    bool
	reachable atomic.Bool
	checked   atomic.Bool
}

// Mutated only from the UI goroutine; mu guards hosts for background readers.
type hostTable struct {
	*tview.Table

	mu    sync.Mutex
	hosts []*hostEntry

	frame   int
	filter  string
	visible []*hostEntry
}

func newHostTable(configs []ssh.SSHConfig) *hostTable {
	t := &hostTable{
		Table: tview.NewTable().
			SetBorders(false).
			SetSelectable(true, false).
			SetFixed(1, 0),
	}
	for _, cfg := range configs {
		t.hosts = append(t.hosts, &hostEntry{cfg: cfg})
	}
	t.SetFilter("")
	return t
}

func (t *hostTable) Snapshot() []*hostEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.hosts)
}

func (t *hostTable) Find(host string) *hostEntry {
	for _, h := range t.hosts {
		if h.cfg.Host == host {
			return h
		}
	}
	return nil
}

func (t *hostTable) Add(cfg ssh.SSHConfig) *hostEntry {
	h := &hostEntry{cfg: cfg}
	t.mu.Lock()
	t.hosts = append(t.hosts, h)
	t.mu.Unlock()
	t.SetFilter(t.filter)
	t.SelectHost(h)
	return h
}

func (t *hostTable) Remove(h *hostEntry) {
	row, _ := t.GetSelection()
	t.mu.Lock()
	t.hosts = slices.DeleteFunc(t.hosts, func(e *hostEntry) bool { return e == h })
	t.mu.Unlock()
	t.SetFilter(t.filter)
	t.Select(max(min(row, len(t.visible)), 1), 0)
}

func (t *hostTable) SetFilter(filter string) {
	t.filter = filter
	needle := strings.ToLower(filter)
	t.visible = t.visible[:0]
	for _, h := range t.hosts {
		if strings.Contains(strings.ToLower(h.cfg.Host), needle) ||
			strings.Contains(strings.ToLower(h.cfg.HostName), needle) {
			t.visible = append(t.visible, h)
		}
	}
	t.Render()
	t.Select(1, 0)
	t.ScrollToBeginning()
}

func (t *hostTable) SelectHost(h *hostEntry) {
	if i := slices.Index(t.visible, h); i != -1 {
		t.Select(i+1, 0)
	}
}

func (t *hostTable) Selected() (*hostEntry, bool) {
	row, _ := t.GetSelection()
	if row < 1 || row > len(t.visible) {
		return nil, false
	}
	return t.visible[row-1], true
}

func (t *hostTable) Marked() []ssh.SSHConfig {
	var res []ssh.SSHConfig
	for _, h := range t.hosts {
		if h.marked {
			res = append(res, h.cfg)
		}
	}
	return res
}

func (t *hostTable) ReachableCount() int {
	n := 0
	for _, h := range t.hosts {
		if h.reachable.Load() {
			n++
		}
	}
	return n
}

func (t *hostTable) Pending() bool {
	for _, h := range t.Snapshot() {
		if !h.checked.Load() {
			return true
		}
	}
	return false
}

func (t *hostTable) Render() {
	t.Clear()
	for col, title := range []string{"", "Host", "HostName", "Port", "User", "Auth"} {
		cell := tview.NewTableCell(title).
			SetTextColor(tcell.ColorWhite).
			SetBackgroundColor(tableHeaderBg).
			SetAttributes(tcell.AttrBold).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		if col != colStatus {
			cell.SetExpansion(1)
		}
		t.SetCell(0, col, cell)
	}

	for r, h := range t.visible {
		cfg := h.cfg
		row := r + 1

		status := spinnerFrames[t.frame%len(spinnerFrames)]
		if h.checked.Load() {
			status = "❌"
			if h.reachable.Load() {
				status = "✅"
			}
		}
		t.SetCell(row, colStatus, tview.NewTableCell(status).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter))

		auth := "All"
		if cfg.Password != "" && cfg.IdentityFile == "" {
			auth = "Passwd"
		} else if cfg.IdentityFile != "" {
			auth = "Key"
		}
		for col, text := range []string{cfg.Host, cfg.HostName, cfg.Port, cfg.User, auth} {
			t.SetCell(row, colHost+col, tview.NewTableCell(text).
				SetExpansion(1).
				SetAlign(tview.AlignLeft))
		}
		if h.marked {
			t.GetCell(row, colHost).SetBackgroundColor(tcell.ColorBlue)
		}
	}
}
