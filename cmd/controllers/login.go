package controllers

import (
	"fmt"
	"github.com/ad/domru/pkg/domru"
	"net/http"
	"strconv"
)

func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	ingressPath := r.Header.Get("X-Ingress-Path")

	if r.Method == "POST" {
		if err := h.handlePostLogin(w, r, ingressPath); err != nil {
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

func (h *Handler) handlePostLogin(w http.ResponseWriter, r *http.Request, ingressPath string) error {
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("ParseForm() err: %v", err)
	}

	phone := r.FormValue("phone")
	accounts, err := h.Accounts(&phone)
	if err != nil {
		return fmt.Errorf("login error: %v", err.Error())
	}

	if n, err := strconv.Atoi(phone); err == nil {
		h.Config.Login = n
		if err = h.Config.WriteConfig(); err != nil {
			return fmt.Errorf("error on write config file: %v", err)
		}
	}

	h.UserAccounts = accounts

	data := domru.AccountsPageData{accounts, phone, ingressPath, ""}

	return h.renderTemplate(w, "accounts", data)
}

func (h *Handler) handleGetLogin(w http.ResponseWriter, r *http.Request, ingressPath string) error {
	data := domru.LoginPageData{"", strconv.Itoa(h.Config.Login), ingressPath}

	return h.renderTemplate(w, "login", data)
}
