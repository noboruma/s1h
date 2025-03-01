package config

import (
	"github.com/noboruma/s1h/internal/credentials"
	"github.com/noboruma/s1h/internal/ssh"
)

func PopulateCredentialsToConfig(creds credentials.Credentials, configs []ssh.SSHConfig) []ssh.SSHConfig {
	processed := map[string]struct{}{}
	for i, cfg := range configs {
		cred := creds.Entries[cfg.Host]
		cfg.Password = cred.Password
		if cred.Hostname != "" { // Replace outdated data
			cfg.HostName = cred.Hostname
			cfg.User = cred.User
			cfg.Port = cred.Port
		}
		configs[i] = cfg
		processed[cfg.Host] = struct{}{}
	}
	// Add non existing entries
	for k, added := range creds.Entries {
		if added.Hostname == "" {
			continue
		}
		if _, has := processed[k]; has {
			continue
		}
		configs = append(configs, ssh.SSHConfig{
			Host:         k,
			User:         added.User,
			Port:         added.Port,
			HostName:     added.Hostname,
			IdentityFile: "",
			Password:     added.Password,
		})
	}
	return configs
}
