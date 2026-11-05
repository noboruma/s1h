package ssh

import (
	"encoding/json"
	"fmt"
	"os"
)

var localFileCache *os.File

type SCPHistoryEntry struct {
	From string
	To   string
}

type ExecHistoryEntry struct {
	Command string
}

type HostHistoryEntry struct {
	SCPHistoryEntry
	ExecHistoryEntry
}

var hostsHistory map[string]HostHistoryEntry

func LoadSCPHistory(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		hostsHistory = map[string]HostHistoryEntry{}
		localFileCache, err = os.Create(path)
		return err
	}
	err = json.Unmarshal(b, &hostsHistory)
	if err != nil {
		fmt.Println("[warning] history file broken, using empty one")
		hostsHistory = map[string]HostHistoryEntry{}
		localFileCache, err = os.Create(path)
		return err
	}
	localFileCache, err = os.OpenFile(path, os.O_RDWR, 0o600)
	return err
}

func GetSCPUploadEntry(host string) SCPHistoryEntry {
	return hostsHistory[host+":upload"].SCPHistoryEntry
}

func GetSCPDownloadEntry(host string) SCPHistoryEntry {
	return hostsHistory[host+":download"].SCPHistoryEntry
}

func PutSCPUploadEntry(host string, entry SCPHistoryEntry) {
	putSCPEntry(host+":upload", entry)
}

func PutSCPDownloadEntry(host string, entry SCPHistoryEntry) {
	putSCPEntry(host+":download", entry)
}

func putSCPEntry(key string, entry SCPHistoryEntry) {
	v, has := hostsHistory[key]
	if has && v.SCPHistoryEntry == entry {
		return
	}
	v.SCPHistoryEntry = entry
	hostsHistory[key] = v
	persistHistory()
}

func GetExecEntry(host string) ExecHistoryEntry {
	if e, ok := hostsHistory[host+":exec"]; ok {
		return e.ExecHistoryEntry
	}
	// Exec history used to be stored under the upload key.
	return hostsHistory[host+":upload"].ExecHistoryEntry
}

func PutExecEntry(host string, entry ExecHistoryEntry) {
	key := host + ":exec"
	v, has := hostsHistory[key]
	if has && v.ExecHistoryEntry == entry {
		return
	}
	v.ExecHistoryEntry = entry
	hostsHistory[key] = v
	persistHistory()
}

func persistHistory() {
	b, err := json.Marshal(hostsHistory)
	if err == nil {
		err = localFileCache.Truncate(0)
	}
	if err == nil {
		_, err = localFileCache.WriteAt(b, 0)
	}
	if err == nil {
		localFileCache.Sync()
	}
	// Best effort: printing here would corrupt the TUI.
}
