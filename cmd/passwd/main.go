package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/noboruma/s1h/internal/credentials"
)

func main() {
	createKeyCmd := flag.NewFlagSet("create-key", flag.ExitOnError)
	updateCmd := flag.NewFlagSet("upsert", flag.ExitOnError)
	removeCmd := flag.NewFlagSet("remove", flag.ExitOnError)

	var hostname, password string
	updateCmd.StringVar(&hostname, "hostname", "", "The hostname to update")
	updateCmd.StringVar(&password, "password", "", "The password to set for the hostname")

	removeCmd.StringVar(&hostname, "hostname", "", "The hostname to remove")

	if len(os.Args) < 2 {
		fmt.Println("Expected 'create-key', 'update' or 'remove' subcommands")
		os.Exit(1)
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		fmt.Println("Cannot access config dir: ", err.Error())
		os.Exit(1)
	}
	masterKeyFile := filepath.Join(configDir, "master.key")
	credsFile := filepath.Join(configDir, "credentials.enc")

	if os.Args[1] == "create-key" {
		createKeyCmd.Parse(os.Args[2:])

		_, err := os.Stat(masterKeyFile)
		if err == nil {
			fmt.Printf("Master key in %s already exists. No operations applied.\n", masterKeyFile)
			os.Exit(1)
		}

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
		return
	}

	key, err := credentials.LoadMasterKey(masterKeyFile)
	if err != nil {
		fmt.Println("Error loading master key:", err)
		os.Exit(1)
	}

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
	}

	fmt.Println("Unknown command. Expected 'create-key', 'update' or 'remove'.")
	os.Exit(1)
}
