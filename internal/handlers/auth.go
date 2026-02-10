package handlers

import (
	"net/http"

	"simple-coredns-manager/internal/auth"

	"github.com/labstack/echo/v4"
)

func (h *Handler) LoginPage(c echo.Context) error {
	// If already authenticated, redirect to dashboard
	cookie, err := c.Cookie(auth.CookieName)
	if err == nil && cookie.Value != "" {
		return c.Redirect(http.StatusSeeOther, "/")
	}

	pd := PageData{
		Title:     "Login",
		CSRFToken: csrfToken(c),
	}
	return c.Render(http.StatusOK, "login", pd)
}

func (h *Handler) LoginSubmit(c echo.Context) error {
	password := c.FormValue("password")
	if password == "" || !auth.VerifyPassword(password, h.Config.MasterPasswordHash) {
		pd := PageData{
			Title:      "Login",
			CSRFToken:  csrfToken(c),
			FlashError: "Invalid password",
		}
		return c.Render(http.StatusUnauthorized, "login", pd)
	}

	token, err := auth.GenerateToken(h.Config.JWTSecret)
	if err != nil {
		pd := PageData{
			Title:      "Login",
			CSRFToken:  csrfToken(c),
			FlashError: "Failed to create session",
		}
		return c.Render(http.StatusInternalServerError, "login", pd)
	}

	auth.SetCookie(c.Response().Writer, token)
	return c.Redirect(http.StatusSeeOther, "/")
}

func (h *Handler) Logout(c echo.Context) error {
	auth.ClearCookie(c.Response().Writer)
	return c.Redirect(http.StatusSeeOther, "/login")
}
