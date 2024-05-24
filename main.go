package main

import (
	"embed"
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

	tokenProvider := func() string {
		return "token"
	}

	upstream, err := url.Parse("https://myhome.novotelecom.ru")
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(upstream)
	mux := http.NewServeMux()

	addRestPrefixHandler := func(p *httputil.ReverseProxy) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Adding prefix request to %s\n", r.URL)
			r.URL.Path = "/rest/v1" + r.URL.Path
			p.ServeHTTP(w, r)
		}
	}

	defaultProxyHandler := func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Passing request to %s\n", r.URL)
		proxy.ServeHTTP(w, r)
	}

	mux.HandleFunc("/stream", addRestPrefixHandler(proxy))
	mux.HandleFunc("/events", addRestPrefixHandler(proxy))
	mux.HandleFunc("/finances", addRestPrefixHandler(proxy))
	mux.HandleFunc("/", defaultProxyHandler)

	// Modify the request
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = upstream.Scheme
		req.URL.Host = upstream.Host
		req.Host = upstream.Host
		req.Header.Set("Authentication", tokenProvider())
		log.Printf("Proxying request to %s\n", req.URL)
	}

	log.Printf("Listening on %d\n", listenAddr)
	err = http.ListenAndServe(listenAddr, mux)
	if err != nil {
		log.Fatal(err)
	}
}
