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

func findConfig(configs []ssh.SSHConfig, host string) (ssh.SSHConfig, bool) {
	for i := range configs {
		if configs[i].Host == host {
			return configs[i], true
		}
	}
	return ssh.SSHConfig{}, false
}

func Copy(configs []ssh.SSHConfig, left, right string) error {
	leftHost := extractHost(left)
	rightHost := extractHost(right)

	var leftClient, rightClient *cssh.Client
	var leftConfig, rightConfig ssh.SSHConfig
	var err error
	if leftHost != "" { // remote -> local | remote
		var has bool
		leftConfig, has = findConfig(configs, leftHost)
		if !has {
			return fmt.Errorf("config %s not found", leftHost)
		}
		leftClient, err = ssh.SSHClient(leftConfig)
		if err != nil {
			return err
		}

		if rightHost != "" { // remote -> remote
			rightConfig, has = findConfig(configs, rightHost)
			if !has {
				return fmt.Errorf("config %s not found", rightHost)
			}
			rightClient, err = ssh.SSHClient(rightConfig)
			if err != nil {
				return err
			}
			tempPath := filepath.Join(os.TempDir(),
				removeSlashes("s1h"+left+right))
			err = ssh.DownloadFile(leftClient, extractPath(leftHost), tempPath)
			if err != nil {
				return err
			}
			defer os.Remove(tempPath)
			err = ssh.UploadFile(rightClient, tempPath, extractPath(rightHost))
		} else { // remote -> local
			err = ssh.DownloadFile(leftClient, extractPath(leftHost), right)
		}
	} else { // local -> remote
		var has bool
		rightConfig, has = findConfig(configs, rightHost)
		if !has {
			return fmt.Errorf("config %s not found", rightHost)
		}
		rightClient, err = ssh.SSHClient(rightConfig)
		if err != nil {
			return err
		}
		err = ssh.UploadFile(rightClient, left, extractPath(right))
	}
	return err
}

func Shell(configs []ssh.SSHConfig, host string) error {
	cfg, has := findConfig(configs, host)
	if !has {
		return fmt.Errorf("config %s not found", host)
	}
	return ssh.ExecuteSSHShell(cfg)
}
