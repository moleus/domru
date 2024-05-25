package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/domru"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	ingressPath := r.Header.Get("X-Ingress-Path")

	w.Header().Set("Content-Type", "text/html")

	var loginError string

	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			loginError = fmt.Sprintf("ParseForm() err: %v", err)
		} else {
			phone := r.FormValue("phone")
			accounts, err := h.Accounts(&phone)
			if err != nil {
				loginError = fmt.Sprintf("login error: %v", err.Error())
			} else {
				if n, err := strconv.Atoi(phone); err == nil {
					h.Config.Login = n

					if err = h.Config.WriteConfig(); err != nil {
						log.Println("error on write config file ", err)
					}
				}

				h.UserAccounts = accounts
				// log.Printf("got accounts %+v\n", accounts)

				data := domru.AccountsPageData{accounts, phone, ingressPath, loginError}

				var tmpl []byte
				var err error
				if tmpl, err = h.TemplateFs.ReadFile("templates/accounts.html.tmpl"); err != nil {
					fmt.Println(err)
				}

				t := template.New("t")
				t, err = t.Parse(string(tmpl))
				if err != nil {
					loginError = err.Error()
				} else {
					err = t.Execute(w, data)
					if err != nil {
						loginError = err.Error()
					}
				}
			}

			if loginError != "" {
				log.Println(loginError)
			}
			return
		}
	}

	var tmpl []byte
	var err error
	if tmpl, err = h.TemplateFs.ReadFile("templates/login.html.tmpl"); err != nil {
		fmt.Println(err)
	}

	data := domru.LoginPageData{loginError, strconv.Itoa(h.Config.Login), ingressPath}

	t := template.New("t")
	t, err = t.Parse(string(tmpl))
	if err != nil {
		loginError = err.Error()
	} else {
		err = t.Execute(w, data)
		if err != nil {
			loginError = err.Error()
		}
	}

	if loginError != "" {
		log.Println(loginError)
	}
}

func (h *Handler) LoginWithPasswordHandler(w http.ResponseWriter, r *http.Request) {

}

func (h *Handler) LoginAddressHandler(w http.ResponseWriter, r *http.Request) {
	ingressPath := r.Header.Get("X-Ingress-Path")

	// log.Println(r.Method, "/login/address", ingressPath)

	w.Header().Set("Content-Type", "text/html")

	var loginError, phone, index string

	if err := r.ParseForm(); err != nil {
		loginError = fmt.Sprintf("ParseForm() err: %v", err)
	} else {
		phone = r.FormValue("phone")
		index = r.FormValue("index")

		if accountIndex, err := strconv.Atoi(index); err != nil {
			loginError = fmt.Sprintf("ParseForm() err: %v", err)
		} else {
			if accountIndex < 0 || accountIndex > len(h.UserAccounts)-1 {
				loginError = "Selected wrong account"
			} else {
				account := h.UserAccounts[accountIndex]
				h.Account = &account
				result, err := h.RequestCode(&phone, account)
				if err != nil {
					loginError = fmt.Sprintf("loginAddress error: %v", err.Error())
				}

				if n, err := strconv.Atoi(phone); err == nil {
					h.Config.Login = n
				}

				h.Config.Operator = int(h.Account.OperatorID)
				if err = h.Config.WriteConfig(); err != nil {
					log.Println("error on write config file ", err)
				}

				if !result && loginError == "" {
					loginError = "Error on sms send"
				}

			}
		}

	}

	if loginError != "" {
		log.Println(loginError)
	}

	data := domru.SMSPageData{phone, index, ingressPath, loginError}

	var tmpl []byte
	var err error
	if tmpl, err = h.TemplateFs.ReadFile("templates/sms.html.tmpl"); err != nil {
		fmt.Println(err)
	}

	t := template.New("t")
	t, err = t.Parse(string(tmpl))
	if err != nil {
		loginError = err.Error()
	} else {
		err = t.Execute(w, data)
		if err != nil {
			loginError = err.Error()
		}
	}

	if loginError != "" {
		log.Println(loginError)
	}
}

func (h *Handler) RequestCode(username *string, account auth.Account) (result bool, err error) {
	var (
		body   []byte
		client = http.DefaultClient
	)

	url := fmt.Sprintf(domru.API_AUTH_CONFIRMATION, *username)
	// log.Println("/requestCodeHandler", url)

	b, err := json.Marshal(account)
	if err != nil {
		return false, err
	}

	request, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	request = request.WithContext(ctx)

	rt := WithHeader(client.Transport)
	rt.Set("Host", "myhome.novotelecom.ru")
	rt.Set("Content-Type", "application/json")
	rt.Set("User-Agent", domru.API_USER_AGENT)
	rt.Set("Connection", "keep-alive")
	rt.Set("Accept", "*/*")
	rt.Set("Accept-Language", "en-us")
	rt.Set("Accept-Encoding", "gzip, deflate, br")
	rt.Set("Authorization", "")

	client.Transport = rt

	resp, err := client.Do(request)
	if err != nil {
		return false, err
	}

	defer func() {
		err2 := resp.Body.Close()
		if err2 != nil {
			log.Println(err2)
		}
	}()

	if resp.StatusCode == 409 { // Conflict (tokent already expired)
		return false, fmt.Errorf("token can't be refreshed")
	}

	if resp.StatusCode == 200 {
		return true, nil
	}

	if body, err = io.ReadAll(resp.Body); err != nil {
		return false, err
	}

	return false, fmt.Errorf("status %d\n%s", resp.StatusCode, body)
}

// SendCode ...
func (h *Handler) SendCode(r *http.Request) (authToken, refreshToken string, err error) {
	var (
		body   []byte
		client = http.DefaultClient
	)

	url := fmt.Sprintf(domru.API_AUTH_CONFIRMATION_SMS, strconv.Itoa(h.Config.Login))

	if err := r.ParseForm(); err != nil {
		return "", "", fmt.Errorf("ParseForm() err: %v", err)
	}

	code := r.FormValue("code")

	if h.Account.ProfileID == nil {
		return "", "", fmt.Errorf("ProfileID is nil")
	}

	c := auth.ConfirmationRequest{
		Confirm1:     code,
		Confirm2:     code,
		SubscriberID: strconv.Itoa(h.Account.SubscriberID),
		Login:        strconv.Itoa(h.Config.Login),
		OperatorID:   h.Config.Operator,
		ProfileID:    *h.Account.ProfileID,
	}

	b, err := json.Marshal(c)
	if err != nil {
		return "", "", fmt.Errorf("marshal err: %v", err)
	}

	request, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return "", "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	request = request.WithContext(ctx)

	rt := WithHeader(client.Transport)
	rt.Set("Host", "myhome.novotelecom.ru")
	rt.Set("Content-Type", "application/json")
	rt.Set("User-Agent", domru.API_USER_AGENT)
	rt.Set("Connection", "keep-alive")
	rt.Set("Accept", "*/*")
	rt.Set("Accept-Language", "en-us")
	rt.Set("Accept-Encoding", "gzip, deflate, br")
	rt.Set("Authorization", "")

	client.Transport = rt

	resp, err := client.Do(request)
	if err != nil {
		return "", "", err
	}

	defer func() {
		err2 := resp.Body.Close()
		if err2 != nil {
			log.Println(err2)
		}
	}()

	if resp.StatusCode == 409 { // Conflict (tokent already expired)
		return "", "", fmt.Errorf("token can't be refreshed")
	}

	if body, err = io.ReadAll(resp.Body); err != nil {
		return "", "", err
	}

	if resp.StatusCode == 200 {
		var authResp auth.AuthenticationResponse
		if err = json.Unmarshal(body, &authResp); err != nil {
			return "", "", err
		}

		return authResp.AccessToken, authResp.RefreshToken, nil
	}

	return "", "", fmt.Errorf("unknown error with status %d\n%s", resp.StatusCode, body)
}

// LoginSMSHandler ...
func (h *Handler) LoginSMSHandler(w http.ResponseWriter, r *http.Request) {
	// log.Println("/sms")

	access, refresh, err := h.SendCode(r)
	if err != nil {
		log.Println("sms", err.Error())
	}

	h.Config.Token = access
	h.Config.RefreshToken = refresh
	if err = h.Config.WriteConfig(); err != nil {
		log.Println("error on write config file ", err)
	}

	if _, err := w.Write([]byte(access + " / " + refresh)); err != nil {
		log.Println("sms", err.Error())
	}
}
