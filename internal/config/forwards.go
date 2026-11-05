package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/noboruma/s1h/internal/ssh"
)

// SavedForward is a port forward added through s1h's [p] popup. Unlike the
// LocalForward lines of ~/.ssh/config (always on), it can be disabled.
type SavedForward struct {
	ssh.Forward
	Disabled bool `json:"disabled,omitempty"`
}

// ForwardsFile is the JSON file holding SavedForwards by host name.
type ForwardsFile struct{ Path string }

func (f ForwardsFile) Load() (map[string][]SavedForward, error) {
	res := map[string][]SavedForward{}
	b, err := os.ReadFile(f.Path)
	if os.IsNotExist(err) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	return res, json.Unmarshal(b, &res)
}

func (f ForwardsFile) Save(all map[string][]SavedForward) error {
	for k, v := range all {
		if len(v) == 0 {
			delete(all, k)
		}
	}
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(f.Path, append(b, '\n'))
}

func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".s1h-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
