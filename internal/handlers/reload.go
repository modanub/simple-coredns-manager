package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) Reload(c echo.Context) error {
	if err := h.Docker.ReloadCoreDNS(); err != nil {
		setFlash(c, "error", "Reload failed: "+err.Error())
	} else {
		setFlash(c, "success", "CoreDNS reloaded successfully")
	}
	return c.Redirect(http.StatusSeeOther, "/")
}
