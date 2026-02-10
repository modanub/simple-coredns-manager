package handlers

import (
	"net/http"
	"strings"

	"simple-coredns-manager/internal/coredns"

	"github.com/labstack/echo/v4"
)

type HostsListData struct {
	Domains []HostsListEntry
}

type HostsListEntry struct {
	Domain      string
	RecordCount int
}

type HostsEditData struct {
	Domain    string
	Entries   []coredns.HostEntry
	Raw       string
	CSRFToken string
}

type HostsRecordsData struct {
	Domain    string
	Entries   []coredns.HostEntry
	CSRFToken string
}

func (h *Handler) HostsList(c echo.Context) error {
	h.mu.RLock()
	domains, err := h.Hosts.List()
	h.mu.RUnlock()

	var entries []HostsListEntry
	if err == nil {
		for _, d := range domains {
			hf, _ := h.Hosts.Read(d)
			count := 0
			if hf != nil {
				count = len(hf.Entries)
			}
			entries = append(entries, HostsListEntry{Domain: d, RecordCount: count})
		}
	}

	pd := h.page(c, "DNS Records", "hosts", HostsListData{Domains: entries})
	if err != nil {
		pd.FlashError = "Failed to list host files: " + err.Error()
	}
	return c.Render(http.StatusOK, "hosts_list", pd)
}

func (h *Handler) HostsNew(c echo.Context) error {
	pd := h.page(c, "New DNS Zone", "hosts", nil)
	return c.Render(http.StatusOK, "hosts_new", pd)
}

func (h *Handler) HostsEdit(c echo.Context) error {
	domain := c.Param("domain")
	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/hosts")
	}

	h.mu.RLock()
	hf, err := h.Hosts.Read(domain)
	h.mu.RUnlock()
	if err != nil {
		setFlash(c, "error", "Failed to read: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/hosts")
	}

	pd := h.page(c, domain+" — DNS Records", "hosts", HostsEditData{
		Domain:    domain,
		Entries:   hf.Entries,
		Raw:       hf.Raw,
		CSRFToken: csrfToken(c),
	})
	return c.Render(http.StatusOK, "hosts_edit", pd)
}

func (h *Handler) HostsAddEntry(c echo.Context) error {
	domain := c.Param("domain")
	ip := strings.TrimSpace(c.FormValue("ip"))
	hostname := strings.TrimSpace(c.FormValue("hostname"))

	if err := coredns.ValidateDomain(domain); err != nil {
		return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Invalid domain</div>`)
	}
	if ip == "" || hostname == "" {
		return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">IP and hostname are required</div>`)
	}

	h.mu.Lock()
	err := h.Hosts.AddEntry(domain, ip, hostname)
	h.mu.Unlock()
	if err != nil {
		return c.HTML(http.StatusInternalServerError, `<div class="alert alert-danger">Failed to add record: `+err.Error()+`</div>`)
	}

	return h.renderRecordsTable(c, domain)
}

func (h *Handler) HostsRemoveEntry(c echo.Context) error {
	domain := c.Param("domain")
	ip := strings.TrimSpace(c.FormValue("ip"))
	hostname := strings.TrimSpace(c.FormValue("hostname"))

	if err := coredns.ValidateDomain(domain); err != nil {
		return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Invalid domain</div>`)
	}

	h.mu.Lock()
	err := h.Hosts.RemoveEntry(domain, ip, hostname)
	h.mu.Unlock()
	if err != nil {
		return c.HTML(http.StatusInternalServerError, `<div class="alert alert-danger">Failed to delete record: `+err.Error()+`</div>`)
	}

	return h.renderRecordsTable(c, domain)
}

func (h *Handler) renderRecordsTable(c echo.Context, domain string) error {
	h.mu.RLock()
	hf, err := h.Hosts.Read(domain)
	h.mu.RUnlock()

	var entries []coredns.HostEntry
	if err == nil {
		entries = hf.Entries
	}

	data := HostsRecordsData{
		Domain:    domain,
		Entries:   entries,
		CSRFToken: csrfToken(c),
	}
	return c.Render(http.StatusOK, "hosts_records", data)
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
		original = ""
	}

	diff := coredns.GenerateDiff("hosts."+domain, original, newContent)
	return c.Render(http.StatusOK, "hosts_preview", struct{ DiffContent string }{diff})
}

func (h *Handler) HostsSave(c echo.Context) error {
	domain := c.Param("domain")
	content := c.FormValue("content")
	reload := c.FormValue("reload") == "true"

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
			setFlash(c, "warning", "Saved, but reload failed: "+err.Error())
		} else {
			setFlash(c, "success", "Saved and CoreDNS reloaded")
		}
	} else {
		setFlash(c, "success", "Saved successfully")
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

	setFlash(c, "success", "'"+domain+"' deleted")
	return c.Redirect(http.StatusSeeOther, "/hosts")
}
