package config

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	CorefilePath         string
	HostsDir             string
	MasterPasswordHash   []byte
	JWTSecret            []byte
	CoreDNSContainerName string
	Port                 string
}

func Load() (*Config, error) {
	corefilePath := os.Getenv("COREFILE_PATH")
	if corefilePath == "" {
		return nil, fmt.Errorf("COREFILE_PATH is required")
	}

	hostsDir := os.Getenv("HOSTS_DIR")
	if hostsDir == "" {
		return nil, fmt.Errorf("HOSTS_DIR is required")
	}
	if !strings.HasSuffix(hostsDir, "/") {
		hostsDir += "/"
	}

	masterPassword := os.Getenv("MASTER_PASSWORD")
	if masterPassword == "" {
		return nil, fmt.Errorf("MASTER_PASSWORD is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	containerName := os.Getenv("COREDNS_CONTAINER_NAME")
	if containerName == "" {
		containerName = "coredns"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	var passwordHash []byte
	if strings.HasPrefix(masterPassword, "$2a$") || strings.HasPrefix(masterPassword, "$2b$") {
		passwordHash = []byte(masterPassword)
	} else {
		hash, err := bcrypt.GenerateFromPassword([]byte(masterPassword), 12)
		if err != nil {
			return nil, fmt.Errorf("failed to hash master password: %w", err)
		}
		passwordHash = hash
	}

	return &Config{
		CorefilePath:         corefilePath,
		HostsDir:             hostsDir,
		MasterPasswordHash:   passwordHash,
		JWTSecret:            []byte(jwtSecret),
		CoreDNSContainerName: containerName,
		Port:                 port,
	}, nil
}
