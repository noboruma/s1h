package ssh

import (
	"bufio"
	"os"
	"strings"
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
