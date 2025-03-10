package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

//infoPopup(pages, fmt.Sprintf("Error accessing ssh for Host %s: %v",
//	selectedConfigs[i].Host, err))

func InitMultiClients(selectedConfigs []SSHConfig) ([]*cssh.Client, error) {
	clients := make([]*cssh.Client, len(selectedConfigs))
	errs := make([]error, len(selectedConfigs))
	var wg sync.WaitGroup

	wg.Add(len(selectedConfigs))
	for i := range selectedConfigs {
		go func(i int) {
			defer wg.Done()
			client, err := SSHClient(selectedConfigs[i])
			if err != nil {
				errs[i] = err
			} else {
				clients[i] = client
			}
		}(i)
	}
	wg.Wait()
	return clients, errors.Join(errs...)
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

var sshTimeout = 10 * time.Second

func init() {
	customTimeout := os.Getenv("S1H_TIMEOUT_SEC")
	if customTimeout != "" {
		n, err := strconv.Atoi(customTimeout)
		if err != nil {
			log.Fatalf("S1H_TIMEOUT_SEC wrong format: %v", err)
		}
		sshTimeout = time.Duration(n) * time.Second
	}
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
			Timeout:         sshTimeout,
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
			Timeout:         sshTimeout,
		}
	} else {
		def := GetDefaultPrivateKeys()
		for _, auth := range def {
			config = cssh.ClientConfig{
				User:            cfg.User,
				Auth:            []cssh.AuthMethod{auth},
				HostKeyCallback: cssh.InsecureIgnoreHostKey(),
				Timeout:         sshTimeout,
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

	// Clear the screen to get scrollback height
	clearScreen()

	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
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

func UploadFile(client *ssh.Client, localFile, remotePath string, progress ProgressDisplayer) error {
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
			remotePath = filepath.Join(remotePath, filepath.Base(localFile))
		}
	}
	dstFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file %s: %w", remotePath, err)
	}
	defer dstFile.Close()

	if progress != nil {
		stat, _ := srcFile.Stat()
		progress.SetTotalSize(stat.Size())
		_, err = dstFile.ReadFromWithConcurrency(io.TeeReader(srcFile, progress), 0)
	} else {
		_, err = dstFile.ReadFromWithConcurrency(srcFile, 0)
	}
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}

type ProgressDisplayer interface {
	SetTotalSize(int64)
	io.Writer
}

func DownloadFile(client *ssh.Client, remotePath, localFile string, progress ProgressDisplayer) error {
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

	if progress != nil {
		stat, _ := remoteFile.Stat()
		progress.SetTotalSize(stat.Size())
		_, err = io.Copy(localFileHandle, io.TeeReader(remoteFile, progress))
	} else {
		_, err = remoteFile.WriteTo(localFileHandle)
	}
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}

func ExecCommand(client *ssh.Client, command string) ([]byte, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	return sess.CombinedOutput(command)
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}
