package controllers

import (
	"fmt"
	"github.com/ad/domru/cmd/models"
	"github.com/ad/domru/pkg/auth"
	models2 "github.com/ad/domru/pkg/domru/models"
	"net/http"
)

func (h *Handler) SelectAccountHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("ParseForm() err: %v", err), http.StatusInternalServerError)
		return
	}

	phoneNumber := r.FormValue("phone")
	accountId := r.FormValue("accountId")

	accounts, err := h.domruApi.RequestAccounts(phoneNumber)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get user accounts: %v", err), http.StatusInternalServerError)
		return
	}

	var selectedAccount models2.Account
	for _, account := range accounts {
		if account.AccountID == nil {
			continue
		}
		if *account.AccountID == accountId {
			selectedAccount = account
			break
		}
	}

	var loginError = ""
	authenticator := auth.NewPhoneNumberAuthenticator(phoneNumber)
	requestErr := authenticator.RequestSmsCode(selectedAccount)
	if requestErr != nil {
		http.Error(w, fmt.Sprintf("Failed to request confirmation code: %v", err), http.StatusInternalServerError)
		loginError = requestErr.Error()
		return
	}

	data := models.SMSPageData{
		Phone:      phoneNumber,
		BaseUrl:    h.determineBaseUrl(r),
		LoginError: loginError,
	}

	if err = h.renderTemplate(w, "sms", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render confirmation page: %v", err), http.StatusInternalServerError)
		return
	}
}
