package controllers

import (
	"embed"
	"fmt"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/domru"
	"github.com/ad/domru/pkg/domru/constants"
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

	templateFile := fmt.Sprintf("templates/%s.html.tmpl", templateName)
	tmpl, err := h.TemplateFs.ReadFile(templateFile)
	if err != nil {
		return fmt.Errorf("readfile %s: %w", templateFile, err)
	}

	t, err := template.New(templateName).Funcs(getTemplateFunctions()).Parse(string(tmpl))
	if err != nil {
		return fmt.Errorf("parse %s error: %w", templateFile, err)
	}

	err = t.Execute(w, data)
	if err != nil {
		return fmt.Errorf("execute %s error: %w", templateFile, err)
	}

	return nil
}

func getTemplateFunctions() template.FuncMap {
	return template.FuncMap{
		"getSnapshotUrl":     constants.GetSnapshotUrl,
		"getOpenDoorUrl":     constants.GetOpenDoorUrl,
		"getCameraStreamUrl": constants.GetCameraStreamUrl,
	}
}
