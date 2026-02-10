package handlers

import (
	"net/http"

	"simple-coredns-manager/internal/coredns"

	"github.com/labstack/echo/v4"
)

type HostsListData struct {
	Domains []string
}

type HostsEditData struct {
	Domain  string
	Content string
	IsNew   bool
}

type HostsPreviewData struct {
	DiffContent string
}

func (h *Handler) HostsList(c echo.Context) error {
	h.mu.RLock()
	domains, err := h.Hosts.List()
	h.mu.RUnlock()
	if err != nil {
		pd := h.page(c, "Host Files", "hosts", HostsListData{})
		pd.FlashError = "Failed to list host files: " + err.Error()
		return c.Render(http.StatusOK, "hosts_list", pd)
	}

	pd := h.page(c, "Host Files", "hosts", HostsListData{Domains: domains})
	return c.Render(http.StatusOK, "hosts_list", pd)
}

func (h *Handler) HostsNew(c echo.Context) error {
	pd := h.page(c, "New Host File", "hosts", HostsEditData{IsNew: true})
	return c.Render(http.StatusOK, "hosts_new", pd)
}

func (h *Handler) HostsEdit(c echo.Context) error {
	domain := c.Param("domain")
	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/hosts")
	}

	h.mu.RLock()
	content, err := h.Hosts.ReadRaw(domain)
	h.mu.RUnlock()
	if err != nil {
		setFlash(c, "error", "Failed to read host file: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/hosts")
	}

	pd := h.page(c, "Edit "+domain, "hosts", HostsEditData{
		Domain:  domain,
		Content: content,
	})
	return c.Render(http.StatusOK, "hosts_edit", pd)
}

func (h *Handler) HostsPreview(c echo.Context) error {
	domain := c.Param("domain")
	newContent := c.FormValue("content")

	if err := coredns.ValidateDomain(domain); err != nil {
		return c.HTML(http.StatusOK, `<div class="alert alert-danger">Invalid domain</div>`)
	}

	h.mu.RLock()
	original, err := h.Hosts.ReadRaw(domain)
	h.mu.RUnlock()
	if err != nil {
		// New file — diff against empty
		original = ""
	}

	diff := coredns.GenerateDiff(domain, original, newContent)
	data := HostsPreviewData{DiffContent: diff}
	return c.Render(http.StatusOK, "hosts_preview", data)
}

func (h *Handler) HostsSave(c echo.Context) error {
	domain := c.Param("domain")
	content := c.FormValue("content")
	reload := c.FormValue("reload") == "true"

	// For new files, the domain comes from the form
	if domain == "new" {
		domain = c.FormValue("domain")
	}

	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/hosts")
	}

	if content == "" {
		setFlash(c, "error", "Content cannot be empty")
		return c.Redirect(http.StatusSeeOther, "/hosts/"+domain)
	}

	h.mu.Lock()
	err := h.Hosts.Write(domain, content)
	h.mu.Unlock()
	if err != nil {
		setFlash(c, "error", "Failed to save: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/hosts/"+domain)
	}

	if reload {
		if err := h.Docker.ReloadCoreDNS(); err != nil {
			setFlash(c, "warning", "Host file saved, but reload failed: "+err.Error())
		} else {
			setFlash(c, "success", "Host file saved and CoreDNS reloaded")
		}
	} else {
		setFlash(c, "success", "Host file saved")
	}

	return c.Redirect(http.StatusSeeOther, "/hosts/"+domain)
}

func (h *Handler) HostsDelete(c echo.Context) error {
	domain := c.Param("domain")
	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/hosts")
	}

	h.mu.Lock()
	err := h.Hosts.Delete(domain)
	h.mu.Unlock()
	if err != nil {
		setFlash(c, "error", "Failed to delete: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/hosts")
	}

	setFlash(c, "success", "Host file '"+domain+"' deleted")
	return c.Redirect(http.StatusSeeOther, "/hosts")
}
