package ssh

import (
	"path/filepath"
	"testing"
)

func TestHistoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	for range 2 { // second pass reloads an existing file
		if err := LoadSCPHistory(path); err != nil {
			t.Fatal(err)
		}
		PutSCPUploadEntry("h", SCPHistoryEntry{From: "a", To: "b"})
		PutSCPDownloadEntry("h", SCPHistoryEntry{From: "c", To: "d"})
		PutExecEntry("h", ExecHistoryEntry{Command: "uptime"})
		// A shorter rewrite must not leave stale bytes behind.
		PutExecEntry("h", ExecHistoryEntry{Command: "ls"})
	}
	if err := LoadSCPHistory(path); err != nil {
		t.Fatal(err)
	}
	if got := GetSCPUploadEntry("h"); got != (SCPHistoryEntry{"a", "b"}) {
		t.Errorf("upload: %+v", got)
	}
	if got := GetSCPDownloadEntry("h"); got != (SCPHistoryEntry{"c", "d"}) {
		t.Errorf("download: %+v", got)
	}
	if got := GetExecEntry("h"); got.Command != "ls" {
		t.Errorf("exec: %+v", got)
	}
}
