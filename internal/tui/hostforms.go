package tui

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/noboruma/s1h/internal/config"
	"github.com/noboruma/s1h/internal/credentials"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/rivo/tview"
)

type CredentialStore interface {
	Put(host string, entry credentials.Entry) error
	SetPassword(host, password string) error
	Remove(host string) error
}

// Store keeps the encrypted passwords.
var Store CredentialStore

// HostConfigStore edits Host blocks of ~/.ssh/config.
type HostConfigStore interface {
	UpsertHost(config.HostBlock) error
	RemoveHost(host string) error
}

var HostConfigs HostConfigStore

type hostUI struct {
	app     *tview.Application
	pages   *tview.Pages
	table   *hostTable
	refresh func()
	check   func(*hostEntry)

	fwdState        map[string][]config.SavedForward // s1h's own forwards, by host
	pendingForwards map[string]bool                  // forwards being opened, by host|local
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
	confirm, forwards                  *tview.InputField
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
	var fwds []string
	for _, fw := range cfg.LocalForwards {
		fwds = append(fwds, fw.LFlag())
	}
	f.forwards = input("Forwards: ", strings.Join(fwds, ", "))
	return f
}

func (f hostFields) read(cfg ssh.SSHConfig) (ssh.SSHConfig, error) {
	if f.host != nil {
		cfg.Host = strings.TrimSpace(f.host.GetText())
		if cfg.Host == "" || strings.ContainsAny(cfg.Host, " \t*?") {
			return cfg, fmt.Errorf("Host must be non-empty, without spaces, * or ?")
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
	cfg.LocalForwards = nil
	for _, item := range strings.FieldsFunc(f.forwards.GetText(), func(r rune) bool { return r == ',' || r == ' ' }) {
		fw, err := ssh.ParseLFlag(item)
		if err != nil {
			return cfg, err
		}
		if !slices.Contains(cfg.LocalForwards, fw) {
			cfg.LocalForwards = append(cfg.LocalForwards, fw)
		}
	}
	return cfg, nil
}

func blockOf(cfg ssh.SSHConfig) config.HostBlock {
	return config.HostBlock{
		Host:          cfg.Host,
		HostName:      cfg.HostName,
		Port:          cfg.Port,
		User:          cfg.User,
		LocalForwards: cfg.LocalForwards,
	}
}

// savePassword leaves only the password of cfg in the credentials file, also
// dropping any HostName/User/Port stored there (now in ~/.ssh/config).
// Nothing is touched when there is nothing to change, so hosts without a
// password never need a master key.
func savePassword(old, cfg ssh.SSHConfig) error {
	if cfg.Password == old.Password && !old.CredsEndpoint {
		return nil
	}
	if Store == nil {
		return fmt.Errorf("storing passwords is not available")
	}
	if cfg.Password == "" {
		return Store.Remove(cfg.Host)
	}
	return Store.Put(cfg.Host, credentials.Entry{Password: cfg.Password})
}

func (u *hostUI) editPopup(h *hostEntry) {
	if HostConfigs == nil {
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
		u.closePopup()
		if err := HostConfigs.UpsertHost(blockOf(cfg)); err != nil {
			infoPopup(u.pages, fmt.Sprintf("Error saving %s to ~/.ssh/config: %v", cfg.Host, err))
			return
		}
		cfg.Managed, cfg.CredsEndpoint = false, false
		endpointChanged := cfg.HostName != h.cfg.HostName || cfg.User != h.cfg.User || cfg.Port != h.cfg.Port
		pwErr := savePassword(h.cfg, cfg)
		if pwErr != nil {
			cfg.Password = h.cfg.Password
		}
		h.cfg = cfg
		u.syncSaved(h)
		if endpointChanged {
			h.checked.Store(false)
			u.check(h)
		}
		u.table.Render()
		u.refresh()
		if pwErr != nil {
			infoPopup(u.pages, fmt.Sprintf("Saved %s to ~/.ssh/config, but not its password: %v", cfg.Host, pwErr))
		}
	})
	f.form.AddButton("Cancel", u.closePopup)
	u.showForm(fmt.Sprintf("Edit %s (empty password clears it)", h.cfg.Host), f.form, 17)
}

func (u *hostUI) addPopup() {
	if HostConfigs == nil {
		infoPopup(u.pages, "Adding hosts is not available")
		return
	}
	f := newHostFields(ssh.SSHConfig{Port: "22", User: "root"}, true)
	f.form.AddButton("Add", func() {
		cfg, err := f.read(ssh.SSHConfig{})
		if err != nil {
			infoPopup(u.pages, err.Error())
			return
		}
		if u.table.Find(cfg.Host) != nil {
			infoPopup(u.pages, fmt.Sprintf("Host %s already exists", cfg.Host))
			return
		}
		u.closePopup()
		if err := HostConfigs.UpsertHost(blockOf(cfg)); err != nil {
			infoPopup(u.pages, fmt.Sprintf("Error saving %s to ~/.ssh/config: %v", cfg.Host, err))
			return
		}
		pwErr := savePassword(ssh.SSHConfig{}, cfg)
		if pwErr != nil {
			cfg.Password = ""
		}
		h := u.table.Add(cfg)
		u.syncSaved(h)
		u.table.Render()
		u.check(h)
		u.refresh()
		if pwErr != nil {
			infoPopup(u.pages, fmt.Sprintf("Added %s to ~/.ssh/config, but not its password: %v", cfg.Host, pwErr))
		}
	})
	f.form.AddButton("Cancel", u.closePopup)
	u.showForm("Add host", f.form, 19)
}

func (u *hostUI) removePopup(h *hostEntry) {
	if HostConfigs == nil {
		infoPopup(u.pages, "Removing hosts is not available")
		return
	}
	what := "Its Host block in ~/.ssh/config"
	if h.cfg.Managed {
		what = "Its s1h entry"
	}
	confirm := tview.NewModal().
		SetText(fmt.Sprintf("Remove %s (%s)?\n%s, stored password and s1h port forwards will be deleted.",
			h.cfg.Host, h.cfg.HostName, what)).
		AddButtons([]string{"Cancel", "Remove"}).
		SetDoneFunc(func(_ int, label string) {
			u.closePopup()
			if label != "Remove" {
				return
			}
			if !h.cfg.Managed {
				if err := HostConfigs.RemoveHost(h.cfg.Host); err != nil {
					infoPopup(u.pages, fmt.Sprintf("Error removing %s: %v", h.cfg.Host, err))
					return
				}
			}
			var errs []error
			if h.cfg.Password != "" || h.cfg.Managed || h.cfg.CredsEndpoint {
				if Store == nil {
					errs = append(errs, fmt.Errorf("credentials store is not available"))
				} else if err := Store.Remove(h.cfg.Host); err != nil {
					errs = append(errs, err)
				}
			}
			if err := u.dropHost(h); err != nil {
				errs = append(errs, err)
			}
			u.table.Remove(h)
			u.refresh()
			if err := errors.Join(errs...); err != nil {
				infoPopup(u.pages, fmt.Sprintf("Removed %s, but: %v", h.cfg.Host, err))
			}
		})
	u.pages.AddPage("popup", confirm, false, true)
}
