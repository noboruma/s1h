package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/noboruma/s1h/internal/credentials"
	"github.com/noboruma/s1h/internal/ssh"
	"github.com/noboruma/s1h/internal/tui"
)

var Version string

const (
	masterKeyFileName = "master.key"
	credsFileName     = "credentials.enc"
)

func main() {
	showVersion := flag.Bool("version", false, "Display the version number")

	flag.Parse()
	if *showVersion {
		fmt.Println("Version:", Version)
		return
	}

	if len(os.Args) == 1 {
		startMainTUI()
	} else {
		updateCmd := flag.NewFlagSet("upsert", flag.ExitOnError)
		removeCmd := flag.NewFlagSet("remove", flag.ExitOnError)

		var hostname, password string
		updateCmd.StringVar(&hostname, "hostname", "", "The hostname to update")
		updateCmd.StringVar(&password, "password", "", "The password to set for the hostname")

		removeCmd.StringVar(&hostname, "hostname", "", "The hostname to remove")

		key, credsFile := createAndLoadLocalEncryptedFile()
		switch os.Args[1] {
		case "upsert":
			updateCmd.Parse(os.Args[2:])
			if hostname == "" || password == "" {
				fmt.Println("Please provide both hostname and password.")
				os.Exit(1)
			}

			err := credentials.UpsertCredentials(credsFile, hostname, password, key)
			if err != nil {
				fmt.Println("Error updating credentials:", err)
				os.Exit(1)
			}

			fmt.Println("Credentials updated.")
			return
		case "remove":
			removeCmd.Parse(os.Args[2:])
			if hostname == "" {
				fmt.Println("Please provide the hostname to remove.")
				os.Exit(1)
			}

			err := credentials.RemoveCredentials(credsFile, hostname, key)
			if err != nil {
				fmt.Println("Error removing credentials:", err)
				os.Exit(1)
			}

			fmt.Println("Credentials removed.")
			return
		default:
			fmt.Println("Unknown command. Expected 'upsert' or 'remove'.")
			os.Exit(1)
		}
	}
}

func startMainTUI() {
	configPath := os.Getenv("SSH_CONFIG")
	if configPath == "" {
		configPath = os.Getenv("HOME") + "/.ssh/config"
	}

	configs, err := ssh.ParseSSHConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading creds: %v\n", err)
	}

	tui.PopulateAutocompleteCaches(configs)

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
		tui.PopulateCredentialsToConfig(creds, configs)
	}

	tui.DisplaySSHConfig(configs)
}

func createAndLoadLocalEncryptedFile() ([]byte, string) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		fmt.Println("Cannot access config dir: ", err.Error())
		os.Exit(1)
	}
	masterKeyFile := filepath.Join(configDir, "master.key")
	credsFile := filepath.Join(configDir, "credentials.enc")

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
