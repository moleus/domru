package controllers

import (
	"embed"
	"fmt"
	"github.com/ad/domru/pkg/domru"
	"html/template"
	"net/http"
)

type Handler struct {
	domruApi *domru.API

	TemplateFs embed.FS
}

func NewHandlers(templateFs embed.FS) (h *Handler) {
	h = &Handler{
		TemplateFs: templateFs,
	}

	return h
}

func (h *Handler) renderTemplate(w http.ResponseWriter, templateName string, data interface{}) error {
	w.Header().Set("Content-Type", "text/html")

	tmpl, err := h.TemplateFs.ReadFile(fmt.Sprintf("templates/%s.html", templateName))
	if err != nil {
		return fmt.Errorf("readfile templates/%s.html error: %w", templateName, err)
	}

	t := template.New("t")
	t, err = t.Parse(string(tmpl))
	if err != nil {
		return fmt.Errorf("parse templates/%s.html error: %w", templateName, err)
	}

	err = t.Execute(w, data)
	if err != nil {
		return fmt.Errorf("execute templates/%s.html error: %w", templateName, err)
	}

	return nil
}
