package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	VaultAddr  string
	VaultToken string
}

func Load() (*Config, error) {
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		return nil, fmt.Errorf("VAULT_ADDR environment variable is required")
	}

	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		tokenFile := os.Getenv("VAULT_TOKEN_FILE")
		if tokenFile == "" {
			return nil, fmt.Errorf("VAULT_TOKEN or VAULT_TOKEN_FILE environment variable is required")
		}
		data, err := os.ReadFile(tokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read token file: %w", err)
		}
		token = strings.TrimSpace(string(data))
	}

	return &Config{
		VaultAddr:  addr,
		VaultToken: token,
	}, nil
}
