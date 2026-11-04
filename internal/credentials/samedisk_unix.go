//go:build unix

package credentials

import (
	"os"
	"path/filepath"
	"syscall"
)

// Falls back to b's directory when b does not exist yet.
func SameDisk(a, b string) (same, ok bool) {
	devA, okA := deviceOf(a)
	devB, okB := deviceOf(b)
	if !okB {
		devB, okB = deviceOf(filepath.Dir(b))
	}
	if !okA || !okB {
		return false, false
	}
	return devA == devB, true
}

func deviceOf(path string) (uint64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}
