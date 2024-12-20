package controllers

import (
	"fmt"
	"net/http"

	"github.com/moleus/domru/pkg/auth"
)

func (h *Handler) SubmitSmsCodeHandler(w http.ResponseWriter, r *http.Request) {
	phoneNumber := r.FormValue("phone")
	smsCode := r.FormValue("smsCode")

	if h.accountInfo == nil {
		h.Logger.Error("Account info is missing")
		http.Error(w, "Account info is missing", http.StatusInternalServerError)
		return
	}

	authResponse, err := h.domruAPI.SubmitSmsCode(phoneNumber, smsCode, *h.accountInfo)
	if err != nil {
		h.Logger.With("err", err.Error()).Error("Failed to authenticate")
		http.Error(w, fmt.Sprintf("Failed to authenticate: %v", err), http.StatusInternalServerError)
		return
	}

	err = h.credentialsStore.SaveCredentials(auth.NewCredentialsFromAuthResponse(authResponse))
	if err != nil {
		h.Logger.With("err", err.Error()).Error("Failed to save credentials")
		http.Error(w, fmt.Sprintf("Failed to save credentials: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
