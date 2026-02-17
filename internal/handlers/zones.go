package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"simple-coredns-manager/internal/coredns"

	"github.com/labstack/echo/v4"
)

type ZonesListData struct {
	Domains []ZonesListEntry
}

type ZonesListEntry struct {
	Domain      string
	RecordCount int
}

type ZonesEditData struct {
	Domain    string
	Records   []coredns.Record
	SOA       *coredns.SOAData
	Raw       string
	CSRFToken string
}

type ZonesRecordsData struct {
	Domain    string
	Records   []coredns.Record
	CSRFToken string
}

func (h *Handler) ZonesList(c echo.Context) error {
	h.mu.RLock()
	domains, err := h.Zones.List()
	h.mu.RUnlock()

	var entries []ZonesListEntry
	if err == nil {
		for _, d := range domains {
			zf, _ := h.Zones.Read(d)
			count := 0
			if zf != nil {
				count = len(zf.Records)
			}
			entries = append(entries, ZonesListEntry{Domain: d, RecordCount: count})
		}
	}

	pd := h.page(c, "DNS Zones", "zones", ZonesListData{Domains: entries})
	if err != nil {
		pd.FlashError = "Failed to list zone files: " + err.Error()
	}
	return c.Render(http.StatusOK, "zones_list", pd)
}

func (h *Handler) ZonesNew(c echo.Context) error {
	pd := h.page(c, "New DNS Zone", "zones", nil)
	return c.Render(http.StatusOK, "zones_new", pd)
}

func (h *Handler) ZonesEdit(c echo.Context) error {
	domain := c.Param("domain")
	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/zones")
	}

	h.mu.RLock()
	zf, err := h.Zones.Read(domain)
	h.mu.RUnlock()
	if err != nil {
		setFlash(c, "error", "Failed to read: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/zones")
	}

	pd := h.page(c, domain+" — DNS Zone", "zones", ZonesEditData{
		Domain:    domain,
		Records:   zf.Records,
		SOA:       zf.SOA,
		Raw:       zf.Raw,
		CSRFToken: csrfToken(c),
	})
	return c.Render(http.StatusOK, "zones_edit", pd)
}

func (h *Handler) ZonesAddRecord(c echo.Context) error {
	domain := c.Param("domain")
	name := strings.TrimSpace(c.FormValue("name"))
	rtype := strings.TrimSpace(c.FormValue("type"))
	value := strings.TrimSpace(c.FormValue("value"))
	ttlStr := strings.TrimSpace(c.FormValue("ttl"))
	priorityStr := strings.TrimSpace(c.FormValue("priority"))

	if err := coredns.ValidateDomain(domain); err != nil {
		return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Invalid domain</div>`)
	}
	if name == "" || rtype == "" || value == "" {
		return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Name, type, and value are required</div>`)
	}

	var ttl uint32
	if ttlStr != "" {
		t, err := strconv.ParseUint(ttlStr, 10, 32)
		if err != nil {
			return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Invalid TTL</div>`)
		}
		ttl = uint32(t)
	}

	var priority uint16
	if priorityStr != "" && coredns.RecordType(rtype) == coredns.TypeMX {
		p, err := strconv.ParseUint(priorityStr, 10, 16)
		if err != nil {
			return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Invalid priority</div>`)
		}
		priority = uint16(p)
	}

	rec := coredns.Record{
		Name:     name,
		Type:     coredns.RecordType(rtype),
		TTL:      ttl,
		Value:    value,
		Priority: priority,
	}

	h.mu.Lock()
	err := h.Zones.AddRecord(domain, rec)
	h.mu.Unlock()
	if err != nil {
		return c.HTML(http.StatusInternalServerError, `<div class="alert alert-danger">Failed to add record: `+err.Error()+`</div>`)
	}

	return h.renderRecordsTable(c, domain)
}

func (h *Handler) ZonesRemoveRecord(c echo.Context) error {
	domain := c.Param("domain")
	name := strings.TrimSpace(c.FormValue("name"))
	rtype := strings.TrimSpace(c.FormValue("type"))
	value := strings.TrimSpace(c.FormValue("value"))

	if err := coredns.ValidateDomain(domain); err != nil {
		return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Invalid domain</div>`)
	}

	h.mu.Lock()
	err := h.Zones.RemoveRecord(domain, name, coredns.RecordType(rtype), value)
	h.mu.Unlock()
	if err != nil {
		return c.HTML(http.StatusInternalServerError, `<div class="alert alert-danger">Failed to delete record: `+err.Error()+`</div>`)
	}

	return h.renderRecordsTable(c, domain)
}

func (h *Handler) renderRecordsTable(c echo.Context, domain string) error {
	h.mu.RLock()
	zf, err := h.Zones.Read(domain)
	h.mu.RUnlock()

	var records []coredns.Record
	if err == nil {
		records = zf.Records
	}

	data := ZonesRecordsData{
		Domain:    domain,
		Records:   records,
		CSRFToken: csrfToken(c),
	}
	return c.Render(http.StatusOK, "zones_records", data)
}

func (h *Handler) ZonesPreview(c echo.Context) error {
	domain := c.Param("domain")
	newContent := c.FormValue("content")

	if err := coredns.ValidateDomain(domain); err != nil {
		return c.HTML(http.StatusOK, `<div class="alert alert-danger">Invalid domain</div>`)
	}

	h.mu.RLock()
	original, err := h.Zones.ReadRaw(domain)
	h.mu.RUnlock()
	if err != nil {
		original = ""
	}

	diff := coredns.GenerateDiff("db."+domain, original, newContent)
	return c.Render(http.StatusOK, "zones_preview", struct{ DiffContent string }{diff})
}

func (h *Handler) ZonesSave(c echo.Context) error {
	domain := c.Param("domain")
	content := c.FormValue("content")
	reload := c.FormValue("reload") == "true"

	isNew := domain == "new"
	if isNew {
		domain = c.FormValue("domain")
	}

	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/zones")
	}

	h.mu.Lock()
	var err error
	if isNew && content == "" {
		// Creating a new zone with default template
		err = h.Zones.Create(domain)
	} else {
		if content == "" {
			h.mu.Unlock()
			setFlash(c, "error", "Content cannot be empty")
			return c.Redirect(http.StatusSeeOther, "/zones/"+domain)
		}
		// Validate before saving
		if vErr := h.Zones.Validate(domain, content); vErr != nil {
			h.mu.Unlock()
			setFlash(c, "error", "Validation failed: "+vErr.Error())
			return c.Redirect(http.StatusSeeOther, "/zones/"+domain)
		}
		err = h.Zones.Write(domain, content)
	}
	h.mu.Unlock()

	if err != nil {
		setFlash(c, "error", "Failed to save: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/zones/"+domain)
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

	return c.Redirect(http.StatusSeeOther, "/zones/"+domain)
}

func (h *Handler) ZonesDelete(c echo.Context) error {
	domain := c.Param("domain")
	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/zones")
	}

	h.mu.Lock()
	err := h.Zones.Delete(domain)
	h.mu.Unlock()
	if err != nil {
		setFlash(c, "error", "Failed to delete: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/zones")
	}

	setFlash(c, "success", "'"+domain+"' deleted")
	return c.Redirect(http.StatusSeeOther, "/zones")
}
