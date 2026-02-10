package templates

import (
	"html/template"
	"io"
	"strings"

	"github.com/labstack/echo/v4"
)

type Renderer struct {
	templates *template.Template
}

func NewRenderer(templatesDir string) (*Renderer, error) {
	funcMap := template.FuncMap{
		"splitLines": func(s string) []string {
			return strings.Split(s, "\n")
		},
		"hasPrefix": strings.HasPrefix,
		"trimPrefix": strings.TrimPrefix,
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(templatesDir + "/*.html")
	if err != nil {
		return nil, err
	}

	_, err = tmpl.ParseGlob(templatesDir + "/partials/*.html")
	if err != nil {
		return nil, err
	}

	return &Renderer{templates: tmpl}, nil
}

func (r *Renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.templates.ExecuteTemplate(w, name, data)
}
