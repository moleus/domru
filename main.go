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
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"log"
	"net/http"
	"net/url"
)

const credentialsFile = "accounts.json"
const listenAddr = ":8082"

//go:embed templates/*
var templateFs embed.FS

const (
	flagAccessToken     = "token"
	flagRefreshToken    = "refresh"
	flagLogin           = "login"
	flagOperator        = "operator"
	flagPort            = "port"
	flagCredentialsFile = "credentials"
)

func init() {
	pflag.String(flagAccessToken, "", "dom.ru token")
	pflag.String(flagRefreshToken, "", "dom.ru refresh token")
	pflag.Int(flagLogin, 0, "dom.ru login or phone (i.e: 71231234567)")
	pflag.Int(flagOperator, 0, "dom.ru operator")
	pflag.Int(flagPort, 18000, "listen port")
	pflag.String(flagCredentialsFile, "", "credentials file path (i.e: /usr/domofon/credentials.yaml")
	pflag.Parse()

	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		log.Fatalf("Unable to bind flags: %v", err)
	}

	viper.SetEnvPrefix("domru")
	viper.AutomaticEnv()
}

func main() {

	retryableClient := retryablehttp.NewClient()
	retryableClient.RetryMax = 5

	credentialsStore := auth.NewFileCredentialsStore(credentialsFile)
	tokenProvider := token_management.NewValidTokenProvider(credentialsStore)
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
	proxy.Client = authClient
	proxyHandler := proxy.ProxyRequestHandler()

	http.HandleFunc("GET /login", handlers.LoginPageHandler)
	http.HandleFunc("POST /login", handlers.LoginPhoneInputHandler)
	http.HandleFunc("GET /login/address", handlers.SelectAccountHandler)
	http.HandleFunc("POST /loginWithPassword", handlers.LoginWithPasswordHandler)
	http.HandleFunc("POST /sms", handlers.SubmitSmsCodeHandler)
	http.HandleFunc("GET /stream/{cameraId}", handlers.StreamController)
	http.HandleFunc("GET /pages/home.html", checkCredentialsMiddleware(credentialsStore, handlers.HomeHandler))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			log.Printf("Proxying request to %s\n", r.URL)
			proxyHandler(w, r)
		} else {
			http.Redirect(w, r, "/pages/home.html.tmpl", http.StatusMovedPermanently)
		}
	})

	// TODO: add middleware to check if credentials are set and redirect to login page if not

	log.Printf("Listening on %s\n", listenAddr)
	err = http.ListenAndServe(listenAddr, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func checkCredentialsMiddleware(credentialsStore auth.CredentialsStore, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		credentials, err := credentialsStore.LoadCredentials()
		if err != nil || credentials.RefreshToken == "" {
			http.Redirect(w, r, "/pages/login.html.tmpl", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}
