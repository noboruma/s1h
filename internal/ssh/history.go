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

var hostsHistory map[string]SCPHistoryEntry

func LoadSCPHistory(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		hostsHistory = map[string]SCPHistoryEntry{}
		localFileCache, err = os.Create(path)
		return err
	}
	err = json.Unmarshal(b, &hostsHistory)
	if err != nil {
		fmt.Println("[warning] history file broken, using empty one")
		hostsHistory = map[string]SCPHistoryEntry{}
		localFileCache, err = os.Create(path)
		return err
	}
	localFileCache, err = os.Open(path)
	return err
}

func GetSCPUploadEntry(host string) SCPHistoryEntry {
	return hostsHistory[host+":upload"]
}

func GetSCPDownloadEntry(host string) SCPHistoryEntry {
	return hostsHistory[host+":dowload"]
}

func PutSCPUploadEntry(host string, entry SCPHistoryEntry) {
	putSCPEntry(host+":upload", entry)
}

func PutSCPDownloadEntry(host string, entry SCPHistoryEntry) {
	putSCPEntry(host+":download", entry)
}

func putSCPEntry(host string, entry SCPHistoryEntry) {
	v, has := hostsHistory[host]
	if has && v == entry {
		return
	}
	hostsHistory[host] = entry
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
