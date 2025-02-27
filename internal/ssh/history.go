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
	localFileCache, err = os.Open(path)
	return err
}

func GetSCPUploadEntry(host string) SCPHistoryEntry {
	return hostsHistory[host+":upload"].SCPHistoryEntry
}

func GetSCPDownloadEntry(host string) SCPHistoryEntry {
	return hostsHistory[host+":dowload"].SCPHistoryEntry
}

func PutSCPUploadEntry(host string, entry SCPHistoryEntry) {
	putSCPEntry(host+":upload", entry)
}

func PutSCPDownloadEntry(host string, entry SCPHistoryEntry) {
	putSCPEntry(host+":download", entry)
}

func putSCPEntry(host string, entry SCPHistoryEntry) {
	v, has := hostsHistory[host]
	if has && v.SCPHistoryEntry == entry {
		return
	}
	existingEntry := hostsHistory[host]
	existingEntry.SCPHistoryEntry = entry
	hostsHistory[host] = existingEntry
	b, err := json.Marshal(hostsHistory)
	if err != nil {
		fmt.Println("Failed to update local history cache")
		return
	}
	err = localFileCache.Truncate(0)
	if err != nil {
		fmt.Println("Failed to update local history cache")
		return
	}
	_, err = localFileCache.Write(b)
	if err != nil {
		fmt.Println("Failed to update local history cache")
		return
	}
	err = localFileCache.Sync()
	if err != nil {
		fmt.Println("Failed to update local history cache")
		return
	}
}

func GetExecEntry(host string) ExecHistoryEntry {
	return hostsHistory[host+":upload"].ExecHistoryEntry
}

func PutExecEntry(host string, entry ExecHistoryEntry) {
	putExecEntry(host+":upload", entry)
}

func putExecEntry(host string, entry ExecHistoryEntry) {
	v, has := hostsHistory[host]
	if has && v.ExecHistoryEntry == entry {
		return
	}
	existingEntry := hostsHistory[host]
	existingEntry.ExecHistoryEntry = entry
	hostsHistory[host] = existingEntry
	b, err := json.Marshal(hostsHistory)
	if err != nil {
		fmt.Println("Failed to update local history cache")
		return
	}
	err = localFileCache.Truncate(0)
	if err != nil {
		fmt.Println("Failed to update local history cache")
		return
	}
	_, err = localFileCache.Write(b)
	if err != nil {
		fmt.Println("Failed to update local history cache")
		return
	}
	err = localFileCache.Sync()
	if err != nil {
		fmt.Println("Failed to update local history cache")
		return
	}
}
