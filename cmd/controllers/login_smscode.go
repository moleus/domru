package controllers

import (
	"fmt"
	"net/http"

	"github.com/moleus/domru/pkg/auth"
)

func (h *Handler) SubmitSmsCodeHandler(w http.ResponseWriter, r *http.Request) {
	phoneNumber := r.FormValue("phone")
	smsCode := r.FormValue("smsCode")

	authResponse, err := h.domruAPI.SubmitSmsCode(phoneNumber, smsCode)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to authenticate: %v", err), http.StatusInternalServerError)
		return
	}

	err = h.credentialsStore.SaveCredentials(auth.NewCredentialsFromAuthResponse(authResponse))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save credentials: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
