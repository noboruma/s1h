package ssh

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	cssh "golang.org/x/crypto/ssh"
)

type SSHConfig struct {
	Host         string
	User         string
	Port         string
	HostName     string
	IdentityFile string
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

func LoadIdentifyFile(privateKeyPath string) (cssh.Signer, error) {
	privateKeyPath, err := expandTilde(privateKeyPath)
	if err != nil {
		return nil, err
	}
	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		log.Fatalf("Failed to open private key file: %v", err)
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

func ExecuteSSHShell(endpoint, user, password, identityFile string) error {

	os.Stdout.Write([]byte{'\n'})

	var config cssh.ClientConfig

	if password != "" {
		config = cssh.ClientConfig{
			User: user,
			Auth: []cssh.AuthMethod{
				cssh.Password(password),
			},
			HostKeyCallback: cssh.InsecureIgnoreHostKey(),
		}
	} else if identityFile != "" {
		auth, err := LoadIdentifyFile(identityFile)
		if err != nil {
			return err
		}
		config = cssh.ClientConfig{
			User: user,
			Auth: []cssh.AuthMethod{
				cssh.PublicKeys(auth),
			},
			HostKeyCallback: cssh.InsecureIgnoreHostKey(),
		}
	}

	client, err := cssh.Dial("tcp", endpoint, &config)
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

	sessionStdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	defer sessionStdin.Close()

	go func() {
		reader := bufio.NewReader(os.Stdin)
		defer session.Close()
		for {
			input, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			sessionStdin.Write([]byte(input))
		}
	}()

	err = session.RequestPty("xterm-256color", 80, 40, cssh.TerminalModes{})
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
