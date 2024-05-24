package controllers

import (
	"fmt"
	"github.com/ad/domru/cmd/models"
	"net/http"
)

func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	ingressPath := r.Header.Get("X-Ingress-Path")

	if r.Method == "POST" {
		if err := h.handlePhoneInput(w, r, ingressPath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	if err := h.handleGetLogin(w, r, ingressPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *Handler) LoginWithPasswordHandler(w http.ResponseWriter, r *http.Request) {
	ingressPath := r.Header.Get("X-Ingress-Path")

	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("ParseForm() err: %v", err), http.StatusInternalServerError)
		return
	}

	accountId := r.FormValue("account_id")
	password := r.FormValue("password")

	// send request to domru api
	credentials, err := h.domruApi.LoginWithPassword(accountId, password)
	if err != nil {
		errorMessage := fmt.Sprintf("failed to login with password: %v", err)
		data := models.LoginPageData{LoginError: errorMessage, Phone: "", HassioIngress: ingressPath}
		if err = h.renderTemplate(w, "login", data); err != nil {
			http.Error(w, fmt.Sprintf("failed to render login page: %v", err), http.StatusInternalServerError)
		}
		return
	}

	if err = h.credentialsStore.SaveCredentials(credentials); err != nil {
		http.Error(w, fmt.Sprintf("failed to save credentials: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pages/home.html", http.StatusSeeOther)
}

func (h *Handler) handlePhoneInput(w http.ResponseWriter, r *http.Request, ingressPath string) error {
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("ParseForm() err: %v", err)
	}

	phone := r.FormValue("phone")
	accounts, err := h.domruApi.RequestAccounts(phone)
	if err != nil {
		return fmt.Errorf("failed get accounts for phone %s: %v", phone, err.Error())
	}

	data := models.AccountsPageData{Accounts: accounts, Phone: phone, HassioIngress: ingressPath}

	return h.renderTemplate(w, "accounts", data)
}

func (h *Handler) handleGetLogin(w http.ResponseWriter, r *http.Request, ingressPath string) error {
	data := models.LoginPageData{Phone: "TODO: maybe store phone number", HassioIngress: ingressPath}

	return h.renderTemplate(w, "login", data)
}
