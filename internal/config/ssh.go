package config

import (
	"github.com/noboruma/s1h/internal/credentials"
	"github.com/noboruma/s1h/internal/ssh"
)

func PopulateCredentialsToConfig(creds credentials.Credentials, configs []ssh.SSHConfig) {
	for i, cfg := range configs {
		cfg.Password = creds.Entries[cfg.Host]
		configs[i] = cfg
	}
}
