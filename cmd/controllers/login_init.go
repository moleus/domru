package controllers

import (
	"fmt"
	"net/http"

	"github.com/moleus/domru/cmd/models"
)

func (h *Handler) LoginPageHandler(w http.ResponseWriter, r *http.Request) {
	data := models.LoginPageData{Phone: "TODO: maybe store phone number"}
	data.BaseURL = h.determineBaseURL(r)

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

	phone := r.FormValue("phone")
	accounts, err := h.domruAPI.RequestAccounts(phone)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get user accounts: %v", err), http.StatusInternalServerError)
		return
	}

	data := models.AccountsPageData{Accounts: accounts, Phone: phone}
	data.BaseURL = h.determineBaseURL(r)
	data.LoginError = ""

	err = h.renderTemplate(w, "accounts", data)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to render accounts page: %v", err), http.StatusInternalServerError)
	}
}
