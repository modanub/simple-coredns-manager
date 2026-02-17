package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type DashboardData struct {
	CoreDNSStatus    string
	ContainerID      string
	DockerOK         bool
	ZoneFileCount    int
	ZoneFiles        []string
	CorefileExists   bool
	GSLBZoneCount    int
	GSLBBackendCount int
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

	// List zone files
	h.mu.RLock()
	zones, err := h.Zones.List()
	h.mu.RUnlock()
	if err == nil {
		dd.ZoneFiles = zones
		dd.ZoneFileCount = len(zones)
	}

	// List GSLB zones
	h.mu.RLock()
	gslbEntries, err := h.GSLB.List()
	h.mu.RUnlock()
	if err == nil {
		dd.GSLBZoneCount = len(gslbEntries)
		for _, e := range gslbEntries {
			dd.GSLBBackendCount += e.BackendCount
		}
	}

	pd := h.page(c, "Dashboard", "dashboard", dd)
	return c.Render(http.StatusOK, "dashboard", pd)
}
