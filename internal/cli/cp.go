package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/noboruma/s1h/internal/ssh"
	cssh "golang.org/x/crypto/ssh"
)

func extractHost(endpoint string) string {
	n := strings.Index(endpoint, ":")
	if n == -1 {
		return ""
	}
	return endpoint[:n]
}

func removeSlashes(input string) string {
	return strings.ReplaceAll(input, fmt.Sprintf("%v", os.PathSeparator), "")
}

func extractPath(endpoint string) string {
	n := strings.Index(endpoint, ":")
	if n == -1 {
		return endpoint
	}
	return endpoint[n+1:]
}

func Copy(configs []ssh.SSHConfig, first, second string) error {
	firstHost := extractHost(first)
	secondHost := extractHost(second)

	var leftClient, rightClient *cssh.Client
	var leftConfig, rightConfig ssh.SSHConfig
	var err error
	if firstHost != "" { // remote -> local | remote
		for i := range configs {
			if configs[i].Host == firstHost {
				leftConfig = configs[i]
				break
			}
		}
		leftClient, err = ssh.SSHClient(leftConfig)
		if err != nil {
			return err
		}

		if secondHost != "" { // remote -> remote
			for i := range configs {
				if configs[i].Host == secondHost {
					rightConfig = configs[i]
					break
				}
			}
			rightClient, err = ssh.SSHClient(rightConfig)
			if err != nil {
				return err
			}
			tempPath := filepath.Join(os.TempDir(),
				removeSlashes("s1h"+first+second))
			err = ssh.DownloadFile(leftClient, extractPath(firstHost), tempPath)
			if err != nil {
				return err
			}
			defer os.Remove(tempPath)
			err = ssh.UploadFile(rightClient, tempPath, extractPath(secondHost))
		} else { // remote -> local
			err = ssh.DownloadFile(leftClient, extractPath(firstHost), second)
		}
	} else { // local -> remote
		for i := range configs {
			if configs[i].Host == secondHost {
				rightConfig = configs[i]
				break
			}
		}
		rightClient, err = ssh.SSHClient(rightConfig)
		if err != nil {
			return err
		}
		err = ssh.UploadFile(rightClient, extractPath(firstHost), second)
	}
	return err
}
