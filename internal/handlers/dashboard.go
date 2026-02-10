package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type DashboardData struct {
	CoreDNSStatus  string
	ContainerID    string
	DockerOK       bool
	HostFileCount  int
	HostFiles      []string
	CorefileExists bool
}

func (h *Handler) Dashboard(c echo.Context) error {
	dd := DashboardData{}

	// Check Docker/CoreDNS status
	status, containerID, err := h.Docker.FindContainer()
	if err != nil {
		dd.CoreDNSStatus = "Docker unavailable"
		dd.DockerOK = false
	} else if containerID == "" {
		dd.CoreDNSStatus = "Container not found"
		dd.DockerOK = true
	} else {
		dd.CoreDNSStatus = status
		dd.ContainerID = containerID[:12]
		dd.DockerOK = true
	}

	// Check Corefile
	_, err = h.Corefile.Read()
	dd.CorefileExists = err == nil

	// List host files
	h.mu.RLock()
	hosts, err := h.Hosts.List()
	h.mu.RUnlock()
	if err == nil {
		dd.HostFiles = hosts
		dd.HostFileCount = len(hosts)
	}

	pd := h.page(c, "Dashboard", "dashboard", dd)
	return c.Render(http.StatusOK, "dashboard", pd)
}
