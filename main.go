package main

import (
	"log"
	"time"

	"simple-coredns-manager/internal/auth"
	"simple-coredns-manager/internal/config"
	"simple-coredns-manager/internal/coredns"
	"simple-coredns-manager/internal/docker"
	"simple-coredns-manager/internal/handlers"
	"simple-coredns-manager/internal/templates"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	renderer, err := templates.NewRenderer("templates")
	if err != nil {
		log.Fatalf("Template error: %v", err)
	}

	dockerClient := docker.NewClient(cfg.CoreDNSContainerName)
	if !dockerClient.Available() {
		log.Println("WARNING: Docker socket not available — reload features disabled")
	} else {
		log.Println("Docker socket connected")
	}

	corefileManager := coredns.NewCorefileManager(cfg.CorefilePath)
	zoneManager := coredns.NewZoneManager(cfg.ZoneDir)
	gslbManager := coredns.NewGSLBManager(cfg.GSLBDir)

	h := handlers.NewHandler(cfg, corefileManager, zoneManager, gslbManager, dockerClient)

	e := echo.New()
	e.HideBanner = true
	e.Renderer = renderer

	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		ContextKey:     "csrf",
		TokenLookup:    "form:_csrf,header:X-CSRF-Token",
		CookieName:     "_csrf",
		CookiePath:     "/",
		CookieHTTPOnly: true,
		CookieSameSite: 4, // http.SameSiteStrictMode
	}))

	// Rate limiter for login
	loginLimiter := middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:      rate.Every(time.Second),
				Burst:     5,
				ExpiresIn: 3 * time.Minute,
			},
		),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
	})

	// Public routes
	e.GET("/login", h.LoginPage)
	e.POST("/login", h.LoginSubmit, loginLimiter)

	// Authenticated routes
	authed := e.Group("", auth.Middleware(cfg.JWTSecret))
	authed.POST("/logout", h.Logout)
	authed.GET("/", h.Dashboard)
	authed.GET("/corefile", h.CorefileEdit)
	authed.POST("/corefile/preview", h.CorefilePreview)
	authed.POST("/corefile/save", h.CorefileSave)
	authed.GET("/zones", h.ZonesList)
	authed.GET("/zones/new", h.ZonesNew)
	authed.GET("/zones/:domain", h.ZonesEdit)
	authed.POST("/zones/:domain/preview", h.ZonesPreview)
	authed.POST("/zones/:domain/save", h.ZonesSave)
	authed.POST("/zones/:domain/delete", h.ZonesDelete)
	authed.POST("/zones/:domain/record/add", h.ZonesAddRecord)
	authed.POST("/zones/:domain/record/delete", h.ZonesRemoveRecord)
	authed.GET("/gslb", h.GSLBList)
	authed.GET("/gslb/new", h.GSLBNew)
	authed.POST("/gslb/create", h.GSLBCreate)
	authed.GET("/gslb/:domain", h.GSLBEdit)
	authed.POST("/gslb/:domain/preview", h.GSLBPreview)
	authed.POST("/gslb/:domain/save", h.GSLBSaveRaw)
	authed.POST("/gslb/:domain/delete", h.GSLBDelete)
	authed.POST("/gslb/:domain/record/add", h.GSLBAddRecord)
	authed.POST("/gslb/:domain/record/delete", h.GSLBRemoveRecord)
	authed.POST("/gslb/:domain/record/update", h.GSLBUpdateRecord)
	authed.POST("/gslb/:domain/backend/add", h.GSLBAddBackend)
	authed.POST("/gslb/:domain/backend/delete", h.GSLBRemoveBackend)
	authed.GET("/dig", h.DigPage)
	authed.POST("/dig", h.DigQuery)
	authed.POST("/reload", h.Reload)

	e.Logger.Fatal(e.Start(":" + cfg.Port))
}
