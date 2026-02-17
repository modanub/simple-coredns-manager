package handlers

import (
	"net/http"
	"sync"

	"simple-coredns-manager/internal/config"
	"simple-coredns-manager/internal/coredns"
	"simple-coredns-manager/internal/docker"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	Config   *config.Config
	Corefile *coredns.CorefileManager
	Zones    *coredns.ZoneManager
	GSLB     *coredns.GSLBManager
	Docker   *docker.Client
	mu       sync.RWMutex
}

type PageData struct {
	Title         string
	ActiveNav     string
	Authenticated bool
	CSRFToken     string
	FlashSuccess  string
	FlashError    string
	FlashWarning  string
	Data          interface{}
}

func NewHandler(cfg *config.Config, cf *coredns.CorefileManager, zm *coredns.ZoneManager, gm *coredns.GSLBManager, dc *docker.Client) *Handler {
	return &Handler{
		Config:   cfg,
		Corefile: cf,
		Zones:    zm,
		GSLB:     gm,
		Docker:   dc,
	}
}

func csrfToken(c echo.Context) string {
	if token, ok := c.Get("csrf").(string); ok {
		return token
	}
	return ""
}

func (h *Handler) page(c echo.Context, title, nav string, data interface{}) PageData {
	pd := PageData{
		Title:         title,
		ActiveNav:     nav,
		Authenticated: c.Get("authenticated") != nil,
		CSRFToken:     csrfToken(c),
		Data:          data,
	}

	if sess := getFlash(c, "success"); sess != "" {
		pd.FlashSuccess = sess
	}
	if sess := getFlash(c, "error"); sess != "" {
		pd.FlashError = sess
	}
	if sess := getFlash(c, "warning"); sess != "" {
		pd.FlashWarning = sess
	}

	return pd
}

func setFlash(c echo.Context, kind, message string) {
	c.SetCookie(&http.Cookie{
		Name:     "flash_" + kind,
		Value:    message,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   10,
	})
}

func getFlash(c echo.Context, kind string) string {
	cookie, err := c.Cookie("flash_" + kind)
	if err != nil || cookie.Value == "" {
		return ""
	}
	// Clear the flash
	c.SetCookie(&http.Cookie{
		Name:     "flash_" + kind,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	return cookie.Value
}
