package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

	if r.Method == "POST" {
		h.handlePostLogin(w, r, ingressPath)
		return
	}

	h.handleGetLogin(w, r, ingressPath)
}

func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
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

				data := AccountsPageData{accounts, phone, ingressPath, loginError}

				var tmpl []byte
				var err error
				if tmpl, err = h.TemplateFs.ReadFile("templates/accounts.html"); err != nil {
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
	if tmpl, err = h.TemplateFs.ReadFile("templates/login.html"); err != nil {
		fmt.Println(err)
	}

	data := LoginPageData{loginError, strconv.Itoa(h.Config.Login), ingressPath}

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

func (h *Handler) handlePostLogin(w http.ResponseWriter, r *http.Request, ingressPath string) {
	if err := r.ParseForm(); err != nil {
		log.Println(fmt.Sprintf("ParseForm() err: %v", err))
		return
	}

	phone := r.FormValue("phone")
	accounts, err := h.Accounts(&phone)
	if err != nil {
		log.Println(fmt.Sprintf("login error: %v", err.Error()))
		return
	}

	if n, err := strconv.Atoi(phone); err == nil {
		h.Config.Login = n
		h.Config.WriteConfig()
	}

	h.UserAccounts = accounts
	data := AccountsPageData{accounts, phone, ingressPath, ""}

	tmpl, err := h.TemplateFs.ReadFile("templates/accounts.html")
	if err != nil {
		log.Println(err)
		return
	}

	t, err := template.New("t").Parse(string(tmpl))
	if err != nil {
		log.Println(err)
		return
	}

	t.Execute(w, data)
}

func (h *Handler) handleGetLogin(w http.ResponseWriter, r *http.Request, ingressPath string) {
	tmpl, err := h.TemplateFs.ReadFile("templates/login.html")
	if err != nil {
		log.Println(err)
		return
	}

	data := LoginPageData{"", strconv.Itoa(h.Config.Login), ingressPath}

	t, err := template.New("t").Parse(string(tmpl))
	if err != nil {
		log.Println(err)
		return
	}

	t.Execute(w, data)
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

	data := SMSPageData{phone, index, ingressPath, loginError}

	var tmpl []byte
	var err error
	if tmpl, err = h.TemplateFs.ReadFile("templates/sms.html"); err != nil {
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

// HomeHandler ...
func (h *Handler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	ingressPath := r.Header.Get("X-Ingress-Path")
	// log.Println(r.Method, "/", ingressPath)

	if h.Config.Token == "" || h.Config.RefreshToken == "" {
		http.Redirect(w, r, ingressPath+"/login", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	var loginError string

	hostIP, err2 := h.HANetwork()
	if err2 != nil {
		// loginError = "hostIP got: " + err2.Error()
		hostIP = "localhost"
	}

	var cameras Cameras

	if loginError == "" {
		camerasData, err := h.Cameras()
		if err != nil {
			loginError = "cameras (" + camerasData + ") got " + err.Error()
		} else {
			if err := json.Unmarshal([]byte(camerasData), &cameras); err != nil {
				loginError = "cameras (" + camerasData + ") Unmarshal got " + err.Error()
			}
		}
	}

	var places Places

	if loginError == "" {
		placesData, err := h.Places()
		if err != nil {
			loginError = "places (" + placesData + ") got " + err.Error()
		} else {
			if err := json.Unmarshal([]byte(placesData), &places); err != nil {
				loginError = "places (" + placesData + ") Unmarshal got " + err.Error()
			}
		}
	}

	finances, _ := h.GetFinances()

	// fix: https://github.com/ad/domru/issues/11
	host := r.Host
	if host == "" {
		host = fmt.Sprintf("%s:%s", hostIP, strconv.Itoa(h.Config.Port))
	}
	var scheme string
	scheme = r.URL.Scheme
	if scheme == "" {
		scheme = "http"
	}

	data := HomePageData{
		HassioIngress: ingressPath,
		HostIP:        hostIP,
		Port:          strconv.Itoa(h.Config.Port),
		Host:          host,
		Scheme:        scheme,
		LoginError:    loginError,
		Phone:         strconv.Itoa(h.Config.Login),
		Token:         h.Config.Token,
		RefreshToken:  h.Config.RefreshToken,
		Cameras:       cameras,
		Places:        places,
		Finances:      *finances,
	}

	var tmpl []byte
	var err error
	if tmpl, err = h.TemplateFs.ReadFile("templates/home.html"); err != nil {
		fmt.Println("reafile templates/home.html error", err)
	}

	t := template.New("t")
	t, err = t.Parse(string(tmpl))
	if err != nil {
		loginError = "parse templates/home.html " + err.Error()
	} else {
		err = t.Execute(w, data)
		if err != nil {
			loginError = "execute templates/home.html " + err.Error()
		}
	}

	if loginError != "" {
		log.Println(loginError)
	}
}

func (h *Handler) Accounts(username *string) (a []Account, err error) {
	url := fmt.Sprintf(API_AUTH_LOGIN, *username)
	// log.Println("/accountsHandler", url)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	header := request.Header

	header.Set("Content-Type", "application/json")
	header.Set("Accept", "*/*")
	header.Set("User-Agent", API_USER_AGENT)
	header.Set("Authorization", "")
	header.Set("Accept-Language", "en-us")
	header.Set("Accept-Encoding", "gzip, deflate, br")

	resp, err := h.Client.Do(request)
	if err != nil {
		return nil, err
	}

	defer func() {
		err2 := resp.Body.Close()
		if err2 != nil {
			log.Println(err2)
		}
	}()

	if resp.StatusCode == 409 { // Conflict (tokent already expired)
		return nil, fmt.Errorf("token can't be refreshed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var accounts []Account
	if err = json.Unmarshal(body, &accounts); err != nil {
		return nil, err
	}

	return accounts, nil
}

func (h *Handler) RequestCode(username *string, account Account) (result bool, err error) {
	var (
		body   []byte
		client = http.DefaultClient
	)

	url := fmt.Sprintf(API_AUTH_CONFIRMATION, *username)
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
	rt.Set("User-Agent", API_USER_AGENT)
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

	url := fmt.Sprintf(API_AUTH_CONFIRMATION_SMS, strconv.Itoa(h.Config.Login))

	if err := r.ParseForm(); err != nil {
		return "", "", fmt.Errorf("ParseForm() err: %v", err)
	}

	code := r.FormValue("code")

	if h.Account.ProfileID == nil {
		return "", "", fmt.Errorf("ProfileID is nil")
	}

	c := ConfirmationRequest{
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
	rt.Set("User-Agent", API_USER_AGENT)
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
		var authResp AuthenticationResponse
		if err = json.Unmarshal(body, &authResp); err != nil {
			return "", "", err
		}

		return authResp.AccessToken, authResp.RefreshToken, nil
	}

	return "", "", fmt.Errorf("unknown error with status %d\n%s", resp.StatusCode, body)
}

// AccountsHandler ...
func (h *Handler) AccountsHandler(w http.ResponseWriter, r *http.Request) {
	// log.Println("/accountsHandler")

	login := strconv.Itoa(h.Config.Login)

	data, err := h.Accounts(&login)
	if err != nil {
		log.Println("accountsHandler", err.Error())
	}

	w.Header().Set("Content-Type", "application/json")

	b, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("Error: %s", err)

		return
	}

	if _, err := w.Write(b); err != nil {
		log.Println("accountsHandler", err.Error())
	}
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
