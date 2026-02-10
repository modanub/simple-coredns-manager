package handlers

import (
	"net/http"

	"simple-coredns-manager/internal/coredns"

	"github.com/labstack/echo/v4"
)

type CorefileData struct {
	Content string
}

type CorefilePreviewData struct {
	DiffContent string
}

func (h *Handler) CorefileEdit(c echo.Context) error {
	h.mu.RLock()
	content, err := h.Corefile.Read()
	h.mu.RUnlock()
	if err != nil {
		content = ""
		pd := h.page(c, "Corefile", "corefile", CorefileData{Content: content})
		pd.FlashError = "Failed to read Corefile: " + err.Error()
		return c.Render(http.StatusOK, "corefile", pd)
	}

	pd := h.page(c, "Corefile", "corefile", CorefileData{Content: content})
	return c.Render(http.StatusOK, "corefile", pd)
}

func (h *Handler) CorefilePreview(c echo.Context) error {
	newContent := c.FormValue("content")

	h.mu.RLock()
	original, err := h.Corefile.Read()
	h.mu.RUnlock()
	if err != nil {
		return c.HTML(http.StatusOK, `<div class="alert alert-danger">Failed to read current Corefile</div>`)
	}

	diff := coredns.GenerateDiff("Corefile", original, newContent)
	data := CorefilePreviewData{DiffContent: diff}
	return c.Render(http.StatusOK, "corefile_preview", data)
}

func (h *Handler) CorefileSave(c echo.Context) error {
	content := c.FormValue("content")
	reload := c.FormValue("reload") == "true"

	if err := h.Corefile.Validate(content); err != nil {
		setFlash(c, "error", "Validation failed: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/corefile")
	}

	h.mu.Lock()
	err := h.Corefile.Write(content)
	h.mu.Unlock()
	if err != nil {
		setFlash(c, "error", "Failed to save Corefile: "+err.Error())
		return c.Redirect(http.StatusSeeOther, "/corefile")
	}

	if reload {
		if err := h.Docker.ReloadCoreDNS(); err != nil {
			setFlash(c, "warning", "Corefile saved, but reload failed: "+err.Error())
		} else {
			setFlash(c, "success", "Corefile saved and CoreDNS reloaded")
		}
	} else {
		setFlash(c, "success", "Corefile saved")
	}

	return c.Redirect(http.StatusSeeOther, "/corefile")
}
