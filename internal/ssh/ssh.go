package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	cssh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type SSHConfig struct {
	Host         string
	User         string
	Port         string
	HostName     string
	IdentityFile string
	Password     string
}

func (c SSHConfig) Endpoint() string {
	return fmt.Sprintf("%s:%s", c.HostName, c.Port)
}

func ParseSSHConfig(filePath string) ([]SSHConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var configs []SSHConfig
	var currentConfig SSHConfig
	inHostSection := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "Host ") {
			if inHostSection {
				configs = append(configs, currentConfig)
			}
			// skip if host has * in it
			if strings.Contains(line, "*") {
				continue
			}
			currentConfig = SSHConfig{Host: strings.TrimPrefix(line, "Host ")}
			currentConfig.Port = "22"
			inHostSection = true
		} else if inHostSection {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) < 2 {
				continue
			}
			key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])

			switch key {
			case "User":
				currentConfig.User = value
			case "Port":
				currentConfig.Port = value
			case "HostName", "Hostname", "hostname":
				currentConfig.HostName = value
			case "IdentityFile":
				currentConfig.IdentityFile = value
			}
		}
	}

	if inHostSection {
		configs = append(configs, currentConfig)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return configs, nil
}

func findExistingPrivateKeys() ([]string, error) {
	sshRoot := "~/.ssh"
	if os.Getenv("SSH_HOME") != "" {
		sshRoot = os.Getenv("SSH_HOME")
	}

	root, err := expandTilde(sshRoot)
	if err != nil {
		return nil, err
	}

	var res []string
	filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() {
			for _, v := range []string{"known_hosts", "config", ".pub"} {
				if strings.HasSuffix(path, v) {
					return nil
				}
			}
			res = append(res, path)
		}
		return nil
	})
	return res, nil
}

func GetDefaultPrivateKeys() []cssh.AuthMethod {
	var res []cssh.AuthMethod

	defaultKeys, err := findExistingPrivateKeys()
	if err != nil {
		fmt.Printf("[warning] failed to find private keys: %v\n", err.Error())
		return nil
	}

	for _, key := range defaultKeys {
		auth, err := LoadIdentifyFile(key)
		if err != nil {
			fmt.Printf("[warning] failed to load %s: %v\n", key, err.Error())
			continue
		}
		res = append(res, cssh.PublicKeys(auth))
	}

	return res
}

func LoadIdentifyFile(privateKeyPath string) (cssh.Signer, error) {
	privateKeyPath, err := expandTilde(privateKeyPath)
	if err != nil {
		return nil, err
	}
	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}

	return cssh.ParsePrivateKey(privateKey)
}

func expandTilde(path string) (string, error) {
	if path[0] == '~' {
		usr, err := user.Current()
		if err != nil {
			return "", err
		}
		homeDir := usr.HomeDir
		path = filepath.Join(homeDir, path[1:])
	}
	return path, nil
}

func CheckSSHPort(host string, port int, timeout time.Duration) bool {
	address := fmt.Sprintf("%s:%d", host, port)

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

func SSHClient(cfg SSHConfig) (*cssh.Client, error) {

	var config cssh.ClientConfig
	if cfg.Password != "" {
		config = cssh.ClientConfig{
			User: cfg.User,
			Auth: []cssh.AuthMethod{
				cssh.Password(cfg.Password),
			},
			HostKeyCallback: cssh.InsecureIgnoreHostKey(),
		}
	} else if cfg.IdentityFile != "" {
		auth, err := LoadIdentifyFile(cfg.IdentityFile)
		if err != nil {
			return nil, err
		}
		config = cssh.ClientConfig{
			User: cfg.User,
			Auth: []cssh.AuthMethod{
				cssh.PublicKeys(auth),
			},
			HostKeyCallback: cssh.InsecureIgnoreHostKey(),
		}
	} else {
		def := GetDefaultPrivateKeys()
		for _, auth := range def {
			config = cssh.ClientConfig{
				User:            cfg.User,
				Auth:            []cssh.AuthMethod{auth},
				HostKeyCallback: cssh.InsecureIgnoreHostKey(),
			}
			c, err := cssh.Dial("tcp", cfg.Endpoint(), &config)
			if c != nil {
				return c, err
			}
		}
		return nil, errors.New("no key found")
	}

	return cssh.Dial("tcp", cfg.Endpoint(), &config)
}

func ExecuteSSHShell(cfg SSHConfig) error {

	client, err := SSHClient(cfg)
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// set terminal size based on current window size
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		// on error set default width and height
		width = 80
		height = 40
	}

	err = session.RequestPty("xterm-256color", height, width, cssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	})
	if err != nil {
		return err
	}

	err = session.Shell()
	if err != nil {
		return err
	}

	_ = session.Wait()
	return nil
}

func UploadFile(client *ssh.Client, localFile, remotePath string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	srcFile, err := os.Open(localFile)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer srcFile.Close()

	info, err := sftpClient.Stat(remotePath)
	if err == nil {
		if info.IsDir() {
			remotePath = filepath.Join(localFile, filepath.Base(remotePath))
		}
	}
	dstFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %w", err)
	}
	defer dstFile.Close()

	_, err = dstFile.ReadFromWithConcurrency(srcFile, 0)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}

func DownloadFile(client *ssh.Client, remotePath, localFile string) error {
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()

	info, err := os.Stat(localFile)
	if err == nil {
		if info.IsDir() {
			localFile = filepath.Join(localFile, filepath.Base(remotePath))
		}
	}
	localFileHandle, err := os.Create(localFile)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFileHandle.Close()

	_, err = remoteFile.WriteTo(localFileHandle)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}
