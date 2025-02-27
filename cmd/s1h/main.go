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
)

var Version string

const (
	masterKeyFileName = "master.key"
	credsFileName     = "credentials.enc"
	historyFileName   = "history"
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
		startMainTUI(configs)
	} else {
		key, credsFile := loadOrStoreLocalEncryptedFile()
		switch os.Args[1] {
		case "upsert":
			var hostname, password string
			updateCmd := flag.NewFlagSet("upsert", flag.ExitOnError)
			updateCmd.StringVar(&hostname, "hostname", "", "The hostname to update")
			updateCmd.StringVar(&password, "password", "", "The password to set for the hostname")
			err := updateCmd.Parse(os.Args[2:])
			if err != nil {
				fmt.Println("Error upading credentials:", err.Error())
				os.Exit(1)
			}
			if hostname == "" || password == "" {
				fmt.Println("Please provide both hostname and password.")
				os.Exit(1)
			}

			err = credentials.UpsertCredentials(credsFile, hostname, password, key)
			if err != nil {
				fmt.Println("Error updating credentials:", err)
				os.Exit(1)
			}

			fmt.Println("Credentials updated.")
			return
		case "remove":
			var hostname string
			removeCmd := flag.NewFlagSet("remove", flag.ExitOnError)
			removeCmd.StringVar(&hostname, "hostname", "", "The hostname to remove")
			err := removeCmd.Parse(os.Args[2:])
			if err != nil {
				fmt.Println("Error remote:", err)
				os.Exit(1)
			}
			if hostname == "" {
				fmt.Println("Please provide the hostname to remove.")
				os.Exit(1)
			}
			err = credentials.RemoveCredentials(credsFile, hostname, key)
			if err != nil {
				fmt.Println("Error removing credentials:", err.Error())
				os.Exit(1)
			}
			if err != nil {
				fmt.Println("Error removing credentials:", err)
				os.Exit(1)
			}

			fmt.Println("Credentials removed.")
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

func loadConfigs() []ssh.SSHConfig {
	configPath := os.Getenv("SSH_CONFIG")
	if configPath == "" {
		configPath = os.Getenv("HOME") + "/.ssh/config"
	}

	configs, err := ssh.ParseSSHConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading creds: %v\n", err)
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("Error loading creds: %v\n", err)
	}

	masterKeyFile := filepath.Join(configDir, masterKeyFileName)
	credsFile := filepath.Join(configDir, credsFileName)

	var creds credentials.Credentials
	key, err := credentials.LoadMasterKey(masterKeyFile)
	if err == nil {
		creds, err = credentials.LoadCredentials(credsFile, key)
		if err != nil {
			log.Fatalf("Error loading creds: %v\n", err)
		}
		config.PopulateCredentialsToConfig(creds, configs)
	}
	return configs
}

func startMainTUI(configs []ssh.SSHConfig) {

	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("Error loading creds: %v\n", err)
	}

	historyFile := filepath.Join(configDir, historyFileName)

	tui.PopulateAutocompleteCaches(configs)
	err = ssh.LoadSCPHistory(historyFile)
	if err != nil {
		log.Fatalf("Error loading scp history")
	}

	tui.DisplaySSHConfig(configs)
}

func loadOrStoreLocalEncryptedFile() ([]byte, string) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		fmt.Println("Cannot access config dir: ", err.Error())
		os.Exit(1)
	}
	masterKeyFile := filepath.Join(configDir, masterKeyFileName)
	credsFile := filepath.Join(configDir, credsFileName)

	_, err = os.Stat(masterKeyFile)
	if err != nil {
		key, err := credentials.GenerateMasterKey()
		if err != nil {
			fmt.Println("Error generating master key: ", err)
			os.Exit(1)
		}

		err = credentials.SaveMasterKey(masterKeyFile, key)
		if err != nil {
			fmt.Println("Error saving master key: ", err)
			os.Exit(1)
		}

		fmt.Println("Master key saved to", masterKeyFile)
	}

	key, err := credentials.LoadMasterKey(masterKeyFile)
	if err != nil {
		fmt.Println("Error loading master key:", err)
		os.Exit(1)
	}
	return key, credsFile
}
