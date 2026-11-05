package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/noboruma/s1h/internal/ssh"
)

// HostBlock is the part of a "Host" block in ~/.ssh/config that s1h manages.
// Any other line in the block (IdentityFile, comments, ...) is kept as is.
type HostBlock struct {
	Host          string
	HostName      string
	Port          string
	User          string
	LocalForwards []ssh.Forward
}

// UpsertHost updates host's block in the ssh config at path, or adds one.
// New blocks go before the first wildcard Host / Match block: ssh uses the
// first value it finds, so defaults in "Host *" must come after.
func UpsertHost(path string, b HostBlock) error {
	return editConfig(path, func(lines []string) ([]string, error) {
		start, end := findHost(lines, b.Host)
		if start == -1 {
			return insertHost(lines, b), nil
		}
		body := lines[start+1 : end : end]
		body = setKey(body, "HostName", optional(b.HostName))
		body = setKey(body, "Port", optional(b.Port))
		body = setKey(body, "User", optional(b.User))
		var fwds []string
		for _, f := range b.LocalForwards {
			fwds = append(fwds, f.ConfigValue())
		}
		body = setKey(body, "LocalForward", fwds)
		return concat(lines[:start+1], body, lines[end:]), nil
	})
}

// RemoveHost deletes host's block from the ssh config at path. Comments right
// above the next block are kept, as they usually describe that block.
func RemoveHost(path, host string) error {
	return editConfig(path, func(lines []string) ([]string, error) {
		start, end := findHost(lines, host)
		if start == -1 {
			return nil, fmt.Errorf("Host %s not found in %s", host, path)
		}
		cut := end
		for cut > start+1 && isComment(lines[cut-1]) {
			cut--
		}
		// Drop the blank line separating the block too, unless it is still
		// needed in front of what follows.
		blank := func(i int) bool { return strings.TrimSpace(lines[i]) == "" }
		if start > 0 && blank(start-1) && (cut == len(lines) || blank(cut)) {
			start--
		}
		return concat(lines[:start], lines[cut:]), nil
	})
}

func optional(v string) []string {
	if v == "" {
		return nil
	}
	return []string{v}
}

func keyOf(line string) string {
	t := strings.TrimSpace(line)
	if i := strings.IndexAny(t, " \t="); i != -1 {
		t = t[:i]
	}
	return t
}

func isComment(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "#") }

func isSetting(line string) bool {
	return strings.TrimSpace(line) != "" && !isComment(line)
}

func isBlockStart(line string) bool {
	k := keyOf(line)
	return strings.EqualFold(k, "Host") || strings.EqualFold(k, "Match")
}

// findHost returns the index of "Host <host>" and of the line ending its
// block, or -1. Matching is the same as ssh.ParseSSHConfig.
func findHost(lines []string, host string) (start, end int) {
	start = -1
	for i, l := range lines {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "Host ") &&
			strings.TrimSpace(strings.TrimPrefix(t, "Host ")) == host {
			start = i
			break
		}
	}
	if start == -1 {
		return -1, -1
	}
	end = len(lines)
	for i := start + 1; i < len(lines); i++ {
		if isBlockStart(lines[i]) {
			end = i
			break
		}
	}
	return start, end
}

// setKey makes key have exactly values in body: the first existing line of
// key is replaced by them, other lines of key are dropped, and if there was
// none they go after the last setting.
func setKey(body []string, key string, values []string) []string {
	indent := "    "
	for _, l := range body {
		if isSetting(l) {
			indent = l[:len(l)-len(strings.TrimLeft(l, " \t"))]
			break
		}
	}
	var repl []string
	for _, v := range values {
		repl = append(repl, indent+key+" "+v)
	}

	out := []string{}
	at := -1
	for _, l := range body {
		if strings.EqualFold(keyOf(l), key) {
			if at == -1 {
				at = len(out)
			}
			continue
		}
		out = append(out, l)
	}
	if at == -1 {
		at = 0
		for i, l := range out {
			if isSetting(l) {
				at = i + 1
			}
		}
	}
	return concat(out[:at], repl, out[at:])
}

func insertHost(lines []string, b HostBlock) []string {
	block := []string{"Host " + b.Host}
	add := func(key, v string) {
		if v != "" {
			block = append(block, "    "+key+" "+v)
		}
	}
	add("HostName", b.HostName)
	add("Port", b.Port)
	add("User", b.User)
	for _, f := range b.LocalForwards {
		add("LocalForward", f.ConfigValue())
	}

	at := len(lines)
	for i, l := range lines {
		if isBlockStart(l) && (strings.EqualFold(keyOf(l), "Match") || strings.ContainsAny(l, "*?")) {
			at = i
			for at > 0 && isComment(lines[at-1]) {
				at--
			}
			break
		}
	}
	if at < len(lines) {
		block = append(block, "")
	}
	if at > 0 && strings.TrimSpace(lines[at-1]) != "" {
		block = append([]string{""}, block...)
	}
	return concat(lines[:at], block, lines[at:])
}

func concat(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// editConfig runs edit on the lines of the ssh config at path (created if
// missing) and writes the result back atomically.
func editConfig(path string, edit func(lines []string) ([]string, error)) error {
	// Write through symlinks (e.g. a dotfiles-managed config) instead of
	// replacing them.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
	}
	text := string(b)
	var lines []string
	if text != "" {
		lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	}
	lines, err = edit(lines)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")+"\n"))
}
