package controllers

import (
	"fmt"
	"github.com/ad/domru/cmd/models"
	"github.com/ad/domru/pkg/auth"
	"net/http"
)

func (h *Handler) LoginWithPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("ParseForm() err: %v", err), http.StatusInternalServerError)
		return
	}

	accountId := r.FormValue("account_id")
	password := r.FormValue("password")

	authResponse, err := h.domruApi.LoginWithPassword(accountId, password)
	if err != nil {
		h.Logger.Warn("failed to login with password", err.Error())
		errorMessage := fmt.Sprintf("failed to login with password: %v", err)[:100]
		data := models.LoginPageData{LoginError: errorMessage, Phone: ""}
		data.BaseUrl = h.determineBaseUrl(r)
		if err = h.renderTemplate(w, "login", data); err != nil {
			h.Logger.Error("failed to render login page", err.Error())
			http.Error(w, fmt.Sprintf("failed to render login page: %v", err), http.StatusInternalServerError)
		}
		return
	}

	if err = h.credentialsStore.SaveCredentials(auth.NewCredentialsFromAuthResponse(authResponse)); err != nil {
		h.Logger.Error("failed to save credentials", err.Error())
		http.Error(w, fmt.Sprintf("failed to save credentials: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pages/home.html.tmpl", http.StatusSeeOther)
}
