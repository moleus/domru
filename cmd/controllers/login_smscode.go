package controllers

import (
	"fmt"
	"net/http"
)

func (h *Handler) SubmitSmsCodeHandler(w http.ResponseWriter, r *http.Request) {
	phoneNumber := r.FormValue("phone")
	smsCode := r.FormValue("smsCode")

	authResponse, err := h.domruApi.SubmitSmsCode(phoneNumber, smsCode)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to authenticate: %v", err), http.StatusInternalServerError)
		return
	}

	err = h.credentialsStore.SaveCredentials(authResponse)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save credentials: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
