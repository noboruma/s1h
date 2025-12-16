package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

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
	return strings.ReplaceAll(
		strings.ReplaceAll(
			strings.ReplaceAll(input, fmt.Sprintf("%c", os.PathSeparator), "_"),
			"@", "_"),
		":", "_")
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
	progress := progressWriter{startTime: time.Now()}
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
			err = ssh.DownloadFile(leftClient, extractPath(left), tempPath, &progress)
			if err != nil {
				return err
			}
			defer os.Remove(tempPath)
			err = ssh.UploadFile(rightClient, tempPath, extractPath(right), &progress)
		} else { // remote -> local
			err = ssh.DownloadFile(leftClient, extractPath(left), right, &progress)
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
		err = ssh.UploadFile(rightClient, left, extractPath(right), &progress)
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

type progressWriter struct {
	totalBytes       int64
	bytesTransferred atomic.Int64
	startTime        time.Time
}

func (pw *progressWriter) Write(p []byte) (n int, err error) {
	pw.bytesTransferred.Add(int64(len(p)))

	elapsedTime := time.Since(pw.startTime).Seconds()
	transferSpeed := float64(pw.bytesTransferred.Load()) / elapsedTime
	fmt.Printf("\rTransferred: %d/%d bytes (%.2f%%), Speed: %.2f KB/s",
		pw.bytesTransferred.Load(), pw.totalBytes,
		float64(pw.bytesTransferred.Load())/float64(pw.totalBytes)*100,
		transferSpeed/1024)

	return len(p), nil
}
func (pw *progressWriter) SetTotalSize(size int64) {
	pw.totalBytes = size
}
