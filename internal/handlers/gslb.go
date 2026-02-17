package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"simple-coredns-manager/internal/coredns"

	"github.com/labstack/echo/v4"
)

type GSLBListData struct {
	Entries []coredns.GSLBEntry
}

type GSLBEditData struct {
	Domain      string
	Config      *coredns.GSLBConfig
	RecordNames []string
	Raw         string
	CSRFToken   string
}

type GSLBBackendsData struct {
	Domain     string
	RecordName string
	Backends   []coredns.GSLBBackend
	CSRFToken  string
}

func (h *Handler) GSLBList(c echo.Context) error {
	h.mu.RLock()
	entries, err := h.GSLB.List()
	h.mu.RUnlock()

	pd := h.page(c, "GSLB Records", "gslb", GSLBListData{Entries: entries})
	if err != nil {
		pd.FlashError = "Failed to list GSLB configs: " + err.Error()
	}
	return c.Render(http.StatusOK, "gslb_list", pd)
}

func (h *Handler) GSLBNew(c echo.Context) error {
	pd := h.page(c, "New GSLB Zone", "gslb", nil)
	return c.Render(http.StatusOK, "gslb_new", pd)
}

func (h *Handler) GSLBEdit(c echo.Context) error {
	domain := c.Param("domain")
	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb")
	}

	h.mu.RLock()
	cfg, err := h.GSLB.Read(domain)
	raw, _ := h.GSLB.ReadRaw(domain)
	h.mu.RUnlock()
	if err != nil {
		setFlash(c, "error", "Failed to read: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb")
	}

	pd := h.page(c, domain+" — GSLB", "gslb", GSLBEditData{
		Domain:      domain,
		Config:      cfg,
		RecordNames: coredns.SortedRecordNames(cfg.Records),
		Raw:         raw,
		CSRFToken:   csrfToken(c),
	})
	return c.Render(http.StatusOK, "gslb_edit", pd)
}

func (h *Handler) GSLBCreate(c echo.Context) error {
	domain := strings.TrimSpace(c.FormValue("domain"))
	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb/new")
	}

	h.mu.Lock()
	err := h.GSLB.Create(domain)
	h.mu.Unlock()
	if err != nil {
		setFlash(c, "error", "Failed to create: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb/new")
	}

	setFlash(c, "success", "GSLB zone '"+domain+"' created")
	return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
}

func (h *Handler) GSLBDelete(c echo.Context) error {
	domain := c.Param("domain")
	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb")
	}

	h.mu.Lock()
	err := h.GSLB.Delete(domain)
	h.mu.Unlock()
	if err != nil {
		setFlash(c, "error", "Failed to delete: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb")
	}

	setFlash(c, "success", "'"+domain+"' GSLB config deleted")
	return c.Redirect(http.StatusSeeOther, "/gslb")
}

func (h *Handler) GSLBPreview(c echo.Context) error {
	domain := c.Param("domain")
	newContent := c.FormValue("content")

	if err := coredns.ValidateDomain(domain); err != nil {
		return c.HTML(http.StatusOK, `<div class="alert alert-danger">Invalid domain</div>`)
	}

	h.mu.RLock()
	original, err := h.GSLB.ReadRaw(domain)
	h.mu.RUnlock()
	if err != nil {
		original = ""
	}

	diff := coredns.GenerateDiff("db."+domain+".yml", original, newContent)
	return c.Render(http.StatusOK, "gslb_preview", struct{ DiffContent string }{diff})
}

func (h *Handler) GSLBSaveRaw(c echo.Context) error {
	domain := c.Param("domain")
	content := c.FormValue("content")
	reload := c.FormValue("reload") == "true"

	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb")
	}

	if strings.TrimSpace(content) == "" {
		setFlash(c, "error", "Content cannot be empty")
		return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
	}

	h.mu.Lock()
	err := h.GSLB.WriteRaw(domain, content)
	h.mu.Unlock()
	if err != nil {
		setFlash(c, "error", "Failed to save: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
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

	return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
}

func (h *Handler) GSLBAddRecord(c echo.Context) error {
	domain := c.Param("domain")
	recordName := strings.TrimSpace(c.FormValue("record_name"))
	mode := strings.TrimSpace(c.FormValue("mode"))
	ttlStr := strings.TrimSpace(c.FormValue("ttl"))
	scrapeInterval := strings.TrimSpace(c.FormValue("scrape_interval"))

	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
	}

	if recordName == "" || mode == "" {
		setFlash(c, "error", "Record name and mode are required")
		return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
	}

	ttl := 30
	if ttlStr != "" {
		if t, err := strconv.Atoi(ttlStr); err == nil {
			ttl = t
		}
	}

	if scrapeInterval == "" {
		scrapeInterval = "10s"
	}

	h.mu.Lock()
	err := h.GSLB.AddRecord(domain, recordName, mode, ttl, scrapeInterval)
	h.mu.Unlock()
	if err != nil {
		setFlash(c, "error", "Failed to add record: "+err.Error())
	} else {
		setFlash(c, "success", "Record '"+recordName+"' added")
	}

	return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
}

func (h *Handler) GSLBRemoveRecord(c echo.Context) error {
	domain := c.Param("domain")
	recordName := c.FormValue("record_name")

	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
	}

	h.mu.Lock()
	err := h.GSLB.RemoveRecord(domain, recordName)
	h.mu.Unlock()
	if err != nil {
		setFlash(c, "error", "Failed to remove record: "+err.Error())
	} else {
		setFlash(c, "success", "Record removed")
	}

	return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
}

func (h *Handler) GSLBAddBackend(c echo.Context) error {
	domain := c.Param("domain")
	recordName := c.FormValue("record_name")
	address := strings.TrimSpace(c.FormValue("address"))
	priorityStr := strings.TrimSpace(c.FormValue("priority"))
	weightStr := strings.TrimSpace(c.FormValue("weight"))
	location := strings.TrimSpace(c.FormValue("location"))
	healthcheck := strings.TrimSpace(c.FormValue("healthcheck"))

	if err := coredns.ValidateDomain(domain); err != nil {
		return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Invalid domain</div>`)
	}
	if address == "" {
		return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Backend address is required</div>`)
	}

	backend := coredns.GSLBBackend{Address: address}

	if priorityStr != "" {
		if p, err := strconv.Atoi(priorityStr); err == nil {
			backend.Priority = p
		}
	}
	if weightStr != "" {
		if w, err := strconv.Atoi(weightStr); err == nil {
			backend.Weight = w
		}
	}
	if location != "" {
		backend.Location = location
	}
	if healthcheck != "" {
		backend.Healthchecks = []interface{}{healthcheck}
	}

	h.mu.Lock()
	err := h.GSLB.AddBackend(domain, recordName, backend)
	h.mu.Unlock()
	if err != nil {
		return c.HTML(http.StatusInternalServerError, `<div class="alert alert-danger">Failed to add backend: `+err.Error()+`</div>`)
	}

	return h.renderGSLBBackends(c, domain, recordName)
}

func (h *Handler) GSLBRemoveBackend(c echo.Context) error {
	domain := c.Param("domain")
	recordName := c.FormValue("record_name")
	indexStr := c.FormValue("index")

	if err := coredns.ValidateDomain(domain); err != nil {
		return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Invalid domain</div>`)
	}

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return c.HTML(http.StatusBadRequest, `<div class="alert alert-danger">Invalid backend index</div>`)
	}

	h.mu.Lock()
	err = h.GSLB.RemoveBackend(domain, recordName, index)
	h.mu.Unlock()
	if err != nil {
		return c.HTML(http.StatusInternalServerError, `<div class="alert alert-danger">Failed to remove backend: `+err.Error()+`</div>`)
	}

	return h.renderGSLBBackends(c, domain, recordName)
}

func (h *Handler) renderGSLBBackends(c echo.Context, domain, recordName string) error {
	h.mu.RLock()
	cfg, err := h.GSLB.Read(domain)
	h.mu.RUnlock()

	var backends []coredns.GSLBBackend
	if err == nil {
		if rec, ok := cfg.Records[recordName]; ok {
			backends = rec.Backends
		}
	}

	data := GSLBBackendsData{
		Domain:     domain,
		RecordName: recordName,
		Backends:   backends,
		CSRFToken:  csrfToken(c),
	}
	return c.Render(http.StatusOK, "gslb_backends", data)
}

func (h *Handler) GSLBUpdateRecord(c echo.Context) error {
	domain := c.Param("domain")
	recordName := c.FormValue("record_name")
	mode := strings.TrimSpace(c.FormValue("mode"))
	ttlStr := strings.TrimSpace(c.FormValue("ttl"))
	scrapeInterval := strings.TrimSpace(c.FormValue("scrape_interval"))

	if err := coredns.ValidateDomain(domain); err != nil {
		setFlash(c, "error", "Invalid domain: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
	}

	ttl := 30
	if ttlStr != "" {
		if t, err := strconv.Atoi(ttlStr); err == nil {
			ttl = t
		}
	}

	if scrapeInterval == "" {
		scrapeInterval = "10s"
	}

	h.mu.Lock()
	err := h.GSLB.UpdateRecord(domain, recordName, mode, ttl, scrapeInterval)
	h.mu.Unlock()
	if err != nil {
		setFlash(c, "error", "Failed to update: "+err.Error())
	} else {
		setFlash(c, "success", "Record updated")
	}

	return c.Redirect(http.StatusSeeOther, "/gslb/"+domain)
}
