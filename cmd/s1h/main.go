package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/noboruma/s1h/internal/cli"
	"github.com/noboruma/s1h/internal/config"
	"github.com/noboruma/s1h/internal/credentials"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/noboruma/s1h/internal/tui"
	"golang.org/x/crypto/ssh/terminal"
)

var Version string

const (
	masterKeyFileName = "master.key"
	credsFileName     = "credentials.enc"
	historyFileName   = "history"
	forwardsFileName  = "forwards.json"
)

func main() {
	showVersion := flag.Bool("version", false, "Display the version number")

	flag.Parse()
	if *showVersion {
		fmt.Println("Version:", Version)
		return
	}

	if len(os.Args) == 1 {
		configs := loadConfigs()
		tui.Version = Version
		startMainTUI(configs)
	} else {
		key, credsFile := loadOrStoreLocalEncryptedFile()
		switch os.Args[1] {
		case "upsert":
			var host, password, hostname, user, port string
			updateCmd := flag.NewFlagSet("upsert", flag.ExitOnError)
			updateCmd.StringVar(&host, "host", "", "The host to update")
			updateCmd.StringVar(&password, "password", "", "The password to set for the host (optional)")
			updateCmd.StringVar(&hostname, "hostname", "", "The hostname/endpoint to set for the host (optional)")
			updateCmd.StringVar(&user, "user", "root", "The user to use for the host (optional)")
			updateCmd.StringVar(&port, "port", "22", "The port to use for the host (optional)")
			err := updateCmd.Parse(os.Args[2:])
			if err != nil {
				fmt.Println("Error upading credentials:", err.Error())
				os.Exit(1)
			}
			if host == "" {
				fmt.Println("Please provide an host.")
				os.Exit(1)
			}
			if password == "" {
				fmt.Printf("Enter password for %s:", host)
				bytePassword, err := terminal.ReadPassword(int(os.Stdin.Fd()))
				if err != nil {
					fmt.Println("Error reading password:", err)
					return
				}
				password = string(bytePassword)
			}

			err = credentials.UpsertCredential(credsFile, host, hostname, user, port, password, key)
			if err != nil {
				fmt.Println("Error updating credentials:", err)
				os.Exit(1)
			}

			fmt.Println("Credentials updated.")
			return
		case "remove":
			var host string
			removeCmd := flag.NewFlagSet("remove", flag.ExitOnError)
			removeCmd.StringVar(&host, "host", "", "The host to remove")
			err := removeCmd.Parse(os.Args[2:])
			if err != nil {
				fmt.Println("Error remote:", err)
				os.Exit(1)
			}
			if host == "" {
				fmt.Println("Please provide the host to remove.")
				os.Exit(1)
			}
			err = credentials.RemoveCredential(credsFile, host, key)
			if err != nil {
				fmt.Println("Error removing credentials:", err.Error())
				os.Exit(1)
			}
			fmt.Println("Credentials removed.")
			return
		case "reveal":
			var host string
			revealCmd := flag.NewFlagSet("reveal", flag.ExitOnError)
			revealCmd.StringVar(&host, "host", "", "The host to be revealed")
			err := revealCmd.Parse(os.Args[2:])
			if err != nil {
				fmt.Println("Error remote:", err)
				os.Exit(1)
			}
			if host == "" {
				fmt.Println("Please provide the hostname to reveal.")
				os.Exit(1)
			}
			pass, err := credentials.RevealCredentialPassword(credsFile, host, key)
			if err != nil {
				fmt.Println("Error retrieving credential:", err.Error())
				os.Exit(1)
			}
			fmt.Println(pass)
			return
		case "cp":
			if len(os.Args) != 4 || os.Args[2] == "" || os.Args[3] == "" {
				fmt.Println("Missing args: s1h cp [host1:]/path1 [host2:]path2")
				os.Exit(1)
			}
			configs := loadConfigs()
			err := cli.Copy(configs, os.Args[2], os.Args[3])
			if err != nil {
				fmt.Println("Error while copying: ", err.Error())
				os.Exit(1)
			}
		case "shell":
			if len(os.Args) != 3 {
				fmt.Println("Missing args: s1h shell host")
				os.Exit(1)
			}
			configs := loadConfigs()
			err := cli.Shell(configs, os.Args[2])
			if err != nil {
				fmt.Println("Error while copying: ", err.Error())
				os.Exit(1)
			}
		case "ip":
			if len(os.Args) != 3 {
				fmt.Println("Missing args: s1h ip host")
				os.Exit(1)
			}
			configs := loadConfigs()
			for i := range configs {
				if configs[i].Host == os.Args[2] {
					fmt.Printf("%s -> %s\n", configs[i].Host, configs[i].Endpoint())
					os.Exit(0)
				}
			}
			fmt.Printf("Could not find: %s\n", os.Args[2])
			os.Exit(1)
		default:
			fmt.Println("Unknown command. Expected 'upsert', 'remove', 'cp', 'shell', 'ip'.")
			os.Exit(1)
		}
	}
}

func getConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("Error loading creds: %v\n", err)
	}
	configDir = filepath.Join(configDir, "s1h")
	err = os.MkdirAll(configDir, 0700)
	if err != nil {
		log.Fatalf("Error loading creds: %v\n", err)
	}
	_ = os.Chmod(configDir, 0700)
	return configDir
}

func loadConfigs() []ssh.SSHConfig {
	configPath := sshConfigPath()

	configs, err := ssh.ParseSSHConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading creds: %v\n", err)
	}

	configDir := getConfigDir()

	masterKeyFile := masterKeyPath()
	credsFile := filepath.Join(configDir, credsFileName)

	var creds credentials.Credentials
	key, err := credentials.LoadMasterKey(masterKeyFile)
	if err != nil && fileExists(credsFile) {
		warning := fmt.Sprintf("Master key not found at %s: stored passwords are unavailable.\nIs the drive holding it mounted? (see S1H_MASTER_KEY)", masterKeyFile)
		fmt.Fprintln(os.Stderr, warning)
		tui.StartupWarning = warning
	}
	if err == nil {
		creds, err = credentials.LoadCredentials(credsFile, key)
		if err != nil {
			log.Fatalf("Error loading creds: %v\n", err)
		}
		configs = config.PopulateCredentialsToConfig(creds, configs)
	}
	return configs
}

func startMainTUI(configs []ssh.SSHConfig) {

	configDir := getConfigDir()
	historyFile := filepath.Join(configDir, historyFileName)

	err := ssh.LoadSCPHistory(historyFile)
	if err != nil {
		log.Fatalf("Error loading scp history")
	}

	tui.Store = credStore{}
	tui.SavedForwards = config.ForwardsFile{Path: filepath.Join(configDir, forwardsFileName)}
	tui.HostConfigs = sshConfigFile{path: sshConfigPath()}
	tui.MasterKeyLocation = masterKeyLocation
	tui.DisplaySSHConfig(configs)
}

func loadOrStoreLocalEncryptedFile() ([]byte, string) {
	key, credsFile, created, err := ensureMasterKey()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if created {
		fmt.Println("Master key saved to", masterKeyPath())
	}
	if masterKeyLocation() == tui.KeySameDisk {
		fmt.Fprintf(os.Stderr, "Warning: master key %s is on the same disk as %s; anyone reading that disk can decrypt your passwords. Consider moving it to a USB stick (see S1H_MASTER_KEY).\n", masterKeyPath(), credsFile)
	}
	return key, credsFile
}

func ensureMasterKey() (key []byte, credsFile string, created bool, err error) {
	masterKeyFile := masterKeyPath()
	credsFile = filepath.Join(getConfigDir(), credsFileName)

	if !fileExists(masterKeyFile) {
		if fileExists(credsFile) {
			return nil, "", false, fmt.Errorf("master key not found at %s, refusing to create a new one: %s could no longer be decrypted (is the drive holding the key mounted?)", masterKeyFile, credsFile)
		}
		key, err := credentials.GenerateMasterKey()
		if err != nil {
			return nil, "", false, fmt.Errorf("error generating master key: %w", err)
		}
		if err := credentials.SaveMasterKey(masterKeyFile, key); err != nil {
			return nil, "", false, fmt.Errorf("error saving master key: %w", err)
		}
		created = true
	}

	key, err = credentials.LoadMasterKey(masterKeyFile)
	if err != nil {
		return nil, "", false, fmt.Errorf("error loading master key: %w", err)
	}
	return key, credsFile, created, nil
}

type credStore struct{}

func (credStore) Put(host string, entry credentials.Entry) error {
	key, credsFile, _, err := ensureMasterKey()
	if err != nil {
		return err
	}
	return credentials.PutEntry(credsFile, host, entry, key)
}

func (credStore) SetPassword(host, password string) error {
	key, credsFile, _, err := ensureMasterKey()
	if err != nil {
		return err
	}
	return credentials.SetPassword(credsFile, host, password, key)
}

func (credStore) Remove(host string) error {
	key, credsFile, _, err := ensureMasterKey()
	if err != nil {
		return err
	}
	return credentials.RemoveCredential(credsFile, host, key)
}

func sshConfigPath() string {
	if p := os.Getenv("SSH_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(os.Getenv("HOME"), ".ssh", "config")
}

type sshConfigFile struct{ path string }

func (f sshConfigFile) UpsertHost(b config.HostBlock) error { return config.UpsertHost(f.path, b) }
func (f sshConfigFile) RemoveHost(host string) error        { return config.RemoveHost(f.path, host) }

func masterKeyPath() string {
	if p := os.Getenv("S1H_MASTER_KEY"); p != "" {
		return p
	}
	return filepath.Join(getConfigDir(), masterKeyFileName)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func masterKeyLocation() tui.KeyLocation {
	keyFile := masterKeyPath()
	if !fileExists(keyFile) {
		return tui.KeyMissing
	}
	same, ok := credentials.SameDisk(keyFile, filepath.Join(getConfigDir(), credsFileName))
	switch {
	case !ok:
		return tui.KeyUnknown
	case same:
		return tui.KeySameDisk
	default:
		return tui.KeySeparateDisk
	}
}
