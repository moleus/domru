package main

import (
	"embed"
	"github.com/ad/domru/cmd/controllers"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/authorized_sender"
	"github.com/ad/domru/pkg/token_provider"
	"github.com/hashicorp/go-retryablehttp"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

//go:embed templates/*
var templateFs embed.FS

func main() {
	listenAddr := ":8082"
	// Init Config
	//addonConfig := config.InitConfig()

	httpClient := retryablehttp.NewClient()
	httpClient.RetryMax = 5

	credentialsFile := "accounts.json"

	checkTokenUrl := "https://myhome.novotelecom.ru/rest/v1/forpost/server-time"
	credentialsStore := auth.NewFileCredentialsStore(credentialsFile)
	tokenProvider := token_provider.NewValidTokenProvider(credentialsStore, checkTokenUrl)

	authClient := authorized_sender.NewAuthorizedClient(
		tokenProvider,
		authorized_sender.WithClient(httpClient.StandardClient()))

	handlers := controllers.NewHandlers(templateFs)

	upstream, err := url.Parse("https://myhome.novotelecom.ru")
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

	mux.HandleFunc("/pages/home.html", handlers.HomeHandler)

	// TODO: add middleware to check if credentials are set

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
