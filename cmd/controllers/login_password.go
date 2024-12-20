package controllers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/moleus/domru/cmd/models"
	"github.com/moleus/domru/pkg/auth"
	"github.com/moleus/domru/pkg/domru/helpers"
)

func (h *Handler) LoginWithPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, fmt.Sprintf("ParseForm() err: %v", err), http.StatusInternalServerError)
		return
	}

	accountID := r.FormValue("account_id")
	password := r.FormValue("password")

	authResponse, err := h.domruAPI.LoginWithPassword(accountID, password)
	if err != nil {
		h.Logger.With("err", err.Error()).Warn("failed to login with password")

		var errorMessage string
		var upstreamErr *helpers.UpstreamError
		if errors.As(err, &upstreamErr) {
			w.WriteHeader(upstreamErr.StatusCode)
			errorMessage = fmt.Sprintf("upstream returned (hint: retry if 500): %s", upstreamErr.Body)
		} else {
			errorMessage = fmt.Sprintf("Internal error: %s", err.Error())
		}

		data := models.LoginPageData{LoginError: errorMessage, Phone: ""}
		data.BaseURL = h.determineBaseURL(r)
		if err = h.renderTemplate(w, "login", data); err != nil {
			h.Logger.With("err", err.Error()).Error("failed to render login page")
			http.Error(w, fmt.Sprintf("failed to render login page: %v", err), http.StatusInternalServerError)
		}
		return
	}

	if err = h.credentialsStore.SaveCredentials(auth.NewCredentialsFromAuthResponse(authResponse)); err != nil {
		h.Logger.With("err", err.Error()).Error("failed to save credentials")
		http.Error(w, fmt.Sprintf("failed to save credentials: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pages/home.html", http.StatusSeeOther)
}
