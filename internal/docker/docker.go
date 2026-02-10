package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type Client struct {
	containerName string
	available     bool
	cli           *client.Client
}

func NewClient(containerName string) *Client {
	c := &Client{containerName: containerName}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		c.available = false
		return c
	}

	// Quick ping to verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		cli.Close()
		c.available = false
		return c
	}

	c.cli = cli
	c.available = true
	return c
}

func (c *Client) Available() bool {
	return c.available
}

func (c *Client) FindContainer() (status string, containerID string, err error) {
	if !c.available {
		return "", "", fmt.Errorf("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	containers, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", "", fmt.Errorf("failed to list containers: %w", err)
	}

	for _, ctr := range containers {
		for _, name := range ctr.Names {
			// Docker prepends "/" to container names
			cleanName := strings.TrimPrefix(name, "/")
			if cleanName == c.containerName {
				return ctr.State, ctr.ID, nil
			}
		}
	}

	return "", "", nil
}

func (c *Client) ReloadCoreDNS() error {
	if !c.available {
		return fmt.Errorf("Docker not available")
	}

	_, containerID, err := c.FindContainer()
	if err != nil {
		return err
	}
	if containerID == "" {
		return fmt.Errorf("CoreDNS container '%s' not found", c.containerName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// SIGUSR1 triggers CoreDNS to reload its configuration
	return c.cli.ContainerKill(ctx, containerID, "SIGUSR1")
}
