package controllers

import (
	"embed"
	"fmt"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/domru"
	"html/template"
	"net/http"
)

type Handler struct {
	domruApi         *domru.APIWrapper
	credentialsStore auth.CredentialsStore

	TemplateFs embed.FS
}

func NewHandlers(templateFs embed.FS, credentialsStore auth.CredentialsStore, domruApi *domru.APIWrapper) (h *Handler) {
	h = &Handler{
		TemplateFs:       templateFs,
		credentialsStore: credentialsStore,
		domruApi:         domruApi,
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
