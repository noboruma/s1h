package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/noboruma/s1h/internal/credentials"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/rivo/tview"
)

type CredentialStore interface {
	Put(host string, entry credentials.Entry) error
	SetPassword(host, password string) error
	Remove(host string) error
}

var Store CredentialStore

type hostUI struct {
	app     *tview.Application
	pages   *tview.Pages
	table   *hostTable
	refresh func()
	check   func(*hostEntry)
}

func (u *hostUI) closePopup() { u.pages.RemovePage("popup") }

func (u *hostUI) showForm(title string, form *tview.Form, height int) {
	form.SetCancelFunc(u.closePopup)
	form.SetBorder(true).SetTitle(" " + title + " ")
	u.pages.AddPage("popup", modal(form, 64, height), true, true)
}

type hostFields struct {
	form                               *tview.Form
	host, hostname, port, user, passwd *tview.InputField
	confirm                            *tview.InputField
}

func newHostFields(cfg ssh.SSHConfig, withHost bool) hostFields {
	f := hostFields{form: tview.NewForm()}
	input := func(label, value string) *tview.InputField {
		in := tview.NewInputField().SetLabel(label).SetText(value).SetFieldWidth(40)
		f.form.AddFormItem(in)
		return in
	}
	secret := func(label, value string) *tview.InputField {
		in := tview.NewInputField().SetLabel(label).SetText(value).SetFieldWidth(40).SetMaskCharacter('*')
		f.form.AddFormItem(in)
		return in
	}
	if withHost {
		f.host = input("Host: ", cfg.Host)
	}
	f.hostname = input("HostName: ", cfg.HostName)
	f.port = input("Port: ", cfg.Port)
	f.user = input("User: ", cfg.User)
	f.passwd = secret("Password: ", cfg.Password)
	f.confirm = secret("Confirm: ", cfg.Password)
	return f
}

func (f hostFields) read(cfg ssh.SSHConfig) (ssh.SSHConfig, error) {
	if f.host != nil {
		cfg.Host = strings.TrimSpace(f.host.GetText())
		if cfg.Host == "" || strings.ContainsAny(cfg.Host, " \t*") {
			return cfg, fmt.Errorf("Host must be non-empty, without spaces or *")
		}
	}
	cfg.HostName = strings.TrimSpace(f.hostname.GetText())
	cfg.User = strings.TrimSpace(f.user.GetText())
	cfg.Port = strings.TrimSpace(f.port.GetText())
	if cfg.HostName == "" {
		return cfg, fmt.Errorf("HostName is required")
	}
	if p, err := strconv.Atoi(cfg.Port); err != nil || p < 1 || p > 65535 {
		return cfg, fmt.Errorf("Port must be a number between 1 and 65535")
	}
	if f.passwd.GetText() != f.confirm.GetText() {
		return cfg, fmt.Errorf("Passwords do not match")
	}
	cfg.Password = f.passwd.GetText()
	return cfg, nil
}

func entryOf(cfg ssh.SSHConfig) credentials.Entry {
	return credentials.Entry{
		Password: cfg.Password,
		Hostname: cfg.HostName,
		User:     cfg.User,
		Port:     cfg.Port,
	}
}

func (u *hostUI) editPopup(h *hostEntry) {
	if Store == nil {
		infoPopup(u.pages, "Editing hosts is not available")
		return
	}
	f := newHostFields(h.cfg, false)
	f.form.AddButton("Save", func() {
		cfg, err := f.read(h.cfg)
		if err != nil {
			infoPopup(u.pages, err.Error())
			return
		}
		// Only override ~/.ssh/config when the endpoint actually changed.
		endpointChanged := cfg.HostName != h.cfg.HostName || cfg.User != h.cfg.User || cfg.Port != h.cfg.Port
		if h.cfg.Managed || endpointChanged {
			err = Store.Put(cfg.Host, entryOf(cfg))
		} else {
			err = Store.SetPassword(cfg.Host, cfg.Password)
		}
		u.closePopup()
		if err != nil {
			infoPopup(u.pages, fmt.Sprintf("Error saving %s: %v", cfg.Host, err))
			return
		}
		h.cfg = cfg
		if endpointChanged {
			h.checked.Store(false)
			u.check(h)
		}
		u.table.Render()
		u.refresh()
	})
	f.form.AddButton("Cancel", u.closePopup)
	u.showForm(fmt.Sprintf("Edit %s (empty password clears it)", h.cfg.Host), f.form, 15)
}

func (u *hostUI) addPopup() {
	if Store == nil {
		infoPopup(u.pages, "Adding hosts is not available")
		return
	}
	f := newHostFields(ssh.SSHConfig{Port: "22", User: "root"}, true)
	f.form.AddButton("Add", func() {
		cfg, err := f.read(ssh.SSHConfig{Managed: true})
		if err != nil {
			infoPopup(u.pages, err.Error())
			return
		}
		if u.table.Find(cfg.Host) != nil {
			infoPopup(u.pages, fmt.Sprintf("Host %s already exists", cfg.Host))
			return
		}
		u.closePopup()
		if err := Store.Put(cfg.Host, entryOf(cfg)); err != nil {
			infoPopup(u.pages, fmt.Sprintf("Error saving %s: %v", cfg.Host, err))
			return
		}
		u.check(u.table.Add(cfg))
		u.refresh()
	})
	f.form.AddButton("Cancel", u.closePopup)
	u.showForm("Add host", f.form, 17)
}

func (u *hostUI) removePopup(h *hostEntry) {
	if !h.cfg.Managed {
		infoPopup(u.pages, fmt.Sprintf("%s is defined in ~/.ssh/config, remove it there", h.cfg.Host))
		return
	}
	if Store == nil {
		infoPopup(u.pages, "Removing hosts is not available")
		return
	}
	confirm := tview.NewModal().
		SetText(fmt.Sprintf("Remove %s (%s)?\nIts stored credentials will be deleted.", h.cfg.Host, h.cfg.HostName)).
		AddButtons([]string{"Cancel", "Remove"}).
		SetDoneFunc(func(_ int, label string) {
			u.closePopup()
			if label != "Remove" {
				return
			}
			if err := Store.Remove(h.cfg.Host); err != nil {
				infoPopup(u.pages, fmt.Sprintf("Error removing %s: %v", h.cfg.Host, err))
				return
			}
			u.table.Remove(h)
			u.refresh()
		})
	u.pages.AddPage("popup", confirm, false, true)
}
