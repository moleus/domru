package controllers

import (
	"fmt"
	"net/http"

	"github.com/moleus/domru/cmd/models"
	"github.com/moleus/domru/pkg/auth"
	models2 "github.com/moleus/domru/pkg/domru/models"
)

func (h *Handler) SelectAccountHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("ParseForm() err: %v", err), http.StatusInternalServerError)
		return
	}

	phoneNumber := r.FormValue("phone")
	accountID := r.FormValue("accountId")

	accounts, err := h.domruAPI.RequestAccounts(phoneNumber)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get user accounts: %v", err), http.StatusInternalServerError)
		return
	}

	var selectedAccount models2.Account
	for _, account := range accounts {
		if account.AccountID == nil {
			continue
		}
		if *account.AccountID == accountID {
			selectedAccount = account
			break
		}
	}

	authenticator := auth.NewPhoneNumberAuthenticator(phoneNumber)
	requestErr := authenticator.RequestSmsCode(selectedAccount)
	if requestErr != nil {
		http.Error(w, fmt.Sprintf("Failed to request confirmation code: %v", err), http.StatusInternalServerError)
		return
	}

	loginError := ""
	data := models.SMSPageData{
		Phone:      phoneNumber,
		BaseURL:    h.determineBaseURL(r),
		LoginError: loginError,
	}

	if err = h.renderTemplate(w, "sms", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render confirmation page: %v", err), http.StatusInternalServerError)
		return
	}
}
