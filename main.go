package main

import (
	"embed"
	"github.com/ad/domru/cmd/controllers"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/domru"
	"github.com/ad/domru/pkg/domru/constants"
	"github.com/ad/domru/pkg/token_provider"
	"github.com/hashicorp/go-retryablehttp"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

const checkTokenUrl = "https://myhome.novotelecom.ru/rest/v1/forpost/server-time"
const credentialsFile = "accounts.json"
const listenAddr = ":8082"

//go:embed templates/*
var templateFs embed.FS

func main() {

	httpClient := retryablehttp.NewClient()
	httpClient.RetryMax = 5

	credentialsStore := auth.NewFileCredentialsStore(credentialsFile)
	tokenProvider := token_provider.NewValidTokenProvider(credentialsStore, checkTokenUrl)
	authClient := domru.NewAuthorizedClient(
		tokenProvider,
		domru.WithClient(httpClient.StandardClient()))
	domruAPI := domru.NewDomruAPI(authClient)
	handlers := controllers.NewHandlers(templateFs, credentialsStore, domruAPI)

	upstream, err := url.Parse(constants.BaseUrl)
	if err != nil {
		log.Fatal(err)
	}

	proxy := getReverseProxy(upstream, tokenProvider)
	defaultProxyHandler := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Passing request to %s\n", r.URL)
		proxy.ServeHTTP(w, r)
	}

	mux := http.NewServeMux()
	// keep backwards compatibility
	mux.HandleFunc("/stream", addUpstreamAPIPrefix(proxy))
	mux.HandleFunc("/events", addUpstreamAPIPrefix(proxy))
	mux.HandleFunc("/finances", addUpstreamAPIPrefix(proxy))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			defaultProxyHandler(w, r)
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

func getReverseProxy(upstream *url.URL, tokenProvider token_provider.TokenProvider) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.Director = func(req *http.Request) {
		log.Printf("Proxying request to %s\n", req.URL)
		req.URL.Scheme = upstream.Scheme
		req.URL.Host = upstream.Host
		req.Host = upstream.Host
		token, err := tokenProvider.GetToken()
		if err != nil {
			log.Printf("Failed to get token: %s\n", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return proxy
}

func addUpstreamAPIPrefix(proxy *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Adding prefix request to %s\n", r.URL)
		r.URL.Path = "/rest/v1" + r.URL.Path
		proxy.ServeHTTP(w, r)
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
