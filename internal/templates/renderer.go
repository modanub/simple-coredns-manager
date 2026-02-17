package templates

import (
	"fmt"
	"html/template"
	"io"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
)

type Renderer struct {
	templates map[string]*template.Template
}

func NewRenderer(templatesDir string) (*Renderer, error) {
	funcMap := template.FuncMap{
		"splitLines": func(s string) []string {
			return strings.Split(s, "\n")
		},
		"hasPrefix":  strings.HasPrefix,
		"trimPrefix": strings.TrimPrefix,
		"typeBadgeColor": func(t string) string {
			switch t {
			case "A":
				return "primary"
			case "AAAA":
				return "info"
			case "CNAME":
				return "warning"
			case "MX":
				return "success"
			case "TXT":
				return "secondary"
			case "NS":
				return "light"
			default:
				return "dark"
			}
		},
		"map": func(pairs ...interface{}) map[string]interface{} {
			m := make(map[string]interface{})
			for i := 0; i+1 < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if ok {
					m[key] = pairs[i+1]
				}
			}
			return m
		},
	}

	// Parse shared templates (base + partials)
	shared, err := template.New("").Funcs(funcMap).ParseFiles(filepath.Join(templatesDir, "base.html"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse base template: %w", err)
	}
	shared, err = shared.ParseGlob(filepath.Join(templatesDir, "partials", "*.html"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse partials: %w", err)
	}

	// For each page template, clone the shared set and parse the page into it.
	// This prevents {{define "content"}} collisions between pages.
	pages, err := filepath.Glob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob page templates: %w", err)
	}

	templates := make(map[string]*template.Template)
	for _, page := range pages {
		name := strings.TrimSuffix(filepath.Base(page), ".html")
		if name == "base" {
			continue
		}
		clone, err := shared.Clone()
		if err != nil {
			return nil, fmt.Errorf("failed to clone shared templates for %s: %w", name, err)
		}
		t, err := clone.ParseFiles(page)
		if err != nil {
			return nil, fmt.Errorf("failed to parse page template %s: %w", name, err)
		}
		templates[name] = t
	}

	return &Renderer{templates: templates}, nil
}

func (r *Renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	t, ok := r.templates[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	return t.ExecuteTemplate(w, name, data)
}
