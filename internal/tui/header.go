package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var Version string

var StartupWarning string

type KeyLocation int

const (
	KeyMissing KeyLocation = iota
	KeyUnknown
	KeySameDisk
	KeySeparateDisk
)

var MasterKeyLocation func() KeyLocation

var logoLines = []string{
	`      _ _     `,
	`  ___/ | |__  `,
	` / __| | '_ \ `,
	` \__ \ | | | |`,
	` |___/_|_| |_|`,
}

var logoWidth = func() int {
	w := 0
	for _, l := range logoLines {
		w = max(w, len(l))
	}
	return w
}()

func logoText() string {
	var b strings.Builder
	for i, l := range logoLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("[orange::b]" + tview.Escape(l) + "[-::-]")
	}
	return b.String()
}

const headerRows = 6

func headerInfo(shown, total, reachable, selected, forwards, saved int) string {
	version := Version
	if version == "" {
		version = "dev"
	}
	row := func(label, value string) string {
		return fmt.Sprintf(" [orange::b]%-11s[-::-] [white::b]%s[-::-]", label+":", value)
	}
	return strings.Join([]string{
		row("Version", version),
		row("Hosts", hostsCount(shown, total)),
		row("Reachable", fmt.Sprintf("[green::b]%d[-::-]/%d", reachable, total)),
		row("Selected", fmt.Sprintf("[blue::b]%d[-::-]", selected)),
		row("Forwards", fmt.Sprintf("[green::b]%d[-::-] open, %d enabled", forwards, saved)),
		row("Master key", masterKeyStatus()),
	}, "\n")
}

var (
	footerBg      = tcell.PaletteColor(238)
	tableHeaderBg = tcell.PaletteColor(4)
)

var footerHints = []string{
	"[s]hell",
	"[u]pload",
	"[d]ownload",
	"e[x]ec",
	"[p]ort forward",
	"[R]eload forwards",
	"[F]orward all",
	"[space] toggle",
	"[M]ark all/none",
	"[e]dit",
	"[+] add",
	"[-] remove",
	"[r]eveal",
	"[/] filter",
	"[q]uit",
}

func footerText(width int) (text string, lines int) {
	var out []string
	line, lineLen := " ", 1
	for i, hint := range footerHints {
		h := "[#d0d0d0]" + tview.Escape(hint)
		if open, end := strings.Index(hint, "["), strings.Index(hint, "]"); open != -1 && end > open {
			h = "[#d0d0d0]" + tview.Escape(hint[:open]) +
				"[#ffff00::b]" + tview.Escape(hint[open:end+1]) +
				"[#d0d0d0::-]" + tview.Escape(hint[end+1:])
		}
		if i > 0 {
			if width > 0 && lineLen+2+len(hint) > width {
				out = append(out, line)
				line, lineLen = " ", 1
			} else {
				line += "  "
				lineLen += 2
			}
		}
		line += h
		lineLen += len(hint)
	}
	out = append(out, line)
	return strings.Join(out, "\n"), len(out)
}

func hostsCount(shown, total int) string {
	if shown == total {
		return fmt.Sprintf("%d", total)
	}
	return fmt.Sprintf("[yellow::b]%d[-::-]/%d (filtered)", shown, total)
}

func masterKeyStatus() string {
	if MasterKeyLocation == nil {
		return "[gray]unknown[-]"
	}
	switch MasterKeyLocation() {
	case KeyMissing:
		return "[gray]none[-]"
	case KeySameDisk:
		return "[red::b]⚠ same disk as credentials, move it to a USB stick[-::-]"
	case KeySeparateDisk:
		return "[green::b]separate disk[-::-]"
	default:
		return "[gray]unknown[-]"
	}
}
