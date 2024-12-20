package controllers

import (
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/moleus/domru/pkg/auth"
	"github.com/moleus/domru/pkg/domru"
	"github.com/moleus/domru/pkg/domru/constants"
	"github.com/moleus/domru/pkg/homeassistant"
)

type Handler struct {
	Logger           *slog.Logger
	domruAPI         *domru.APIWrapper
	credentialsStore auth.CredentialsStore

	TemplateFs embed.FS
}

func NewHandlers(templateFs embed.FS, credentialsStore auth.CredentialsStore, domruAPI *domru.APIWrapper) (h *Handler) {
	h = &Handler{
		TemplateFs:       templateFs,
		Logger:           slog.Default(),
		credentialsStore: credentialsStore,
		domruAPI:         domruAPI,
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
func (h *Handler) determineBaseURL(r *http.Request) string {
	var scheme string
	var host string

	if scheme = r.URL.Scheme; scheme == "" {
		scheme = "http"
	}
	haHost, haNetworkErr := homeassistant.GetHomeAssistantNetworkAddress()
	if haNetworkErr != nil {
		host = r.Host
	}
	ingressPath := r.Header.Get("X-Ingress-Path")
	if ingressPath == "" && haHost != "" {
		h.Logger.With("ha_host", haHost).Warn("X-Ingress-Path header is empty, when using Home Assistant host")
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, ingressPath)
}
