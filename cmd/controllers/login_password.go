package controllers

import (
	"errors"
	"fmt"
	"github.com/ad/domru/cmd/models"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/domru/helpers"
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

		var errorMessage string
		var upstreamErr *helpers.UpstreamError
		if errors.As(err, &upstreamErr) {
			w.WriteHeader(upstreamErr.StatusCode)
			errorMessage = fmt.Sprintf("upstream returned (hint: retry if 500): %s", upstreamErr.Body)
		} else {
			errorMessage = fmt.Sprintf("Internal error: %s", err.Error())
		}

		data := models.LoginPageData{LoginError: errorMessage, Phone: ""}
		data.BaseUrl = h.determineBaseUrl(r)
		if err = h.renderTemplate(w, "login", data); err != nil {
			h.Logger.Error("failed to render login page", err.Error())
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
