//go:build linux

package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSameDisk(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "master.key")
	if err := os.WriteFile(key, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}

	if same, ok := SameDisk(key, filepath.Join(dir, "credentials.enc")); !ok || !same {
		t.Errorf("same directory: got same=%v ok=%v, want true true", same, ok)
	}
	if same, ok := SameDisk(key, "/proc/self/status"); !ok || same {
		t.Errorf("procfs: got same=%v ok=%v, want false true", same, ok)
	}
	if _, ok := SameDisk(filepath.Join(dir, "missing.key"), key); ok {
		t.Errorf("missing key: got ok=true, want false")
	}
}
