package controllers

import (
	"fmt"
	"github.com/ad/domru/cmd/models"
	"net/http"
)

func (h *Handler) LoginPageHandler(w http.ResponseWriter, r *http.Request) {
	data := models.LoginPageData{Phone: "TODO: maybe store phone number"}
	data.BaseUrl = h.determineBaseUrl(r)

	err := h.renderTemplate(w, "login", data)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to render login page: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handler) LoginPhoneInputHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("ParseForm() err: %v", err), http.StatusInternalServerError)
		return
	}

	var loginError = ""
	phone := r.FormValue("phone")
	accounts, err := h.domruApi.RequestAccounts(phone)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get user accounts: %v", err), http.StatusInternalServerError)
		loginError = err.Error()
		return
	}

	data := models.AccountsPageData{Accounts: accounts, Phone: phone}
	data.BaseUrl = h.determineBaseUrl(r)
	data.LoginError = loginError

	err = h.renderTemplate(w, "accounts", data)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to render accounts page: %v", err), http.StatusInternalServerError)
	}
}
