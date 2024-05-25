package main

import (
	"embed"
	"github.com/ad/domru/cmd/controllers"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/authorizedhttp"
	"github.com/ad/domru/pkg/domru"
	"github.com/ad/domru/pkg/domru/constants"
	"github.com/ad/domru/pkg/reverse_proxy"
	"github.com/ad/domru/pkg/token_management"
	"github.com/hashicorp/go-retryablehttp"
	"log"
	"net/http"
	"net/url"
)

const checkTokenUrl = "https://myhome.novotelecom.ru/rest/v1/forpost/server-time"
const credentialsFile = "accounts.json"
const listenAddr = ":8082"

//go:embed templates/*
var templateFs embed.FS

func main() {
	retryableClient := retryablehttp.NewClient()
	retryableClient.RetryMax = 5

	credentialsStore := auth.NewFileCredentialsStore(credentialsFile)
	tokenProvider := token_management.NewValidTokenProvider(credentialsStore, checkTokenUrl)
	authClient := authorizedhttp.NewClient(
		tokenProvider,
		tokenProvider,
	)
	authClient.DefaultClient = retryableClient.StandardClient()

	domruAPI := domru.NewDomruAPI(authClient)
	handlers := controllers.NewHandlers(templateFs, credentialsStore, domruAPI)

	upstream, err := url.Parse(constants.BaseUrl)
	if err != nil {
		log.Fatal(err)
	}

	proxy := reverse_proxy.NewReverseProxy(upstream)
	proxyHandler := proxy.ProxyRequestHandler()

	mux := http.NewServeMux()
	// keep backwards compatibility
	mux.HandleFunc("/stream", addUpstreamAPIPrefix(proxy))
	mux.HandleFunc("/events", addUpstreamAPIPrefix(proxy))
	mux.HandleFunc("/finances", addUpstreamAPIPrefix(proxy))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			proxyHandler(w, r)
		} else {
			http.Redirect(w, r, "/pages/home.html", http.StatusMovedPermanently)
		}
	})

	mux.HandleFunc("/pages/home.html", checkCredentialsMiddleware(credentialsStore, handlers.HomeHandler))
	mux.HandleFunc("/pages/login.html", handlers.LoginHandler)

	// TODO: add middleware to check if credentials are set and redirect to login page if not

	log.Printf("Listening on %s\n", listenAddr)
	err = http.ListenAndServe(listenAddr, mux)
	if err != nil {
		log.Fatal(err)
	}
}

func addUpstreamAPIPrefix(proxy *reverse_proxy.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Adding prefix request to %s\n", r.URL)
		r.URL.Path = "/rest/v1" + r.URL.Path
		proxy.ProxyRequestHandler()(w, r)
	}
}

func checkCredentialsMiddleware(credentialsStore auth.CredentialsStore, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		credentials, err := credentialsStore.LoadCredentials()
		if err != nil || credentials.RefreshToken == "" {
			http.Redirect(w, r, "/pages/login.html", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}
