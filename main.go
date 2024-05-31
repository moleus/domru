package main

import (
	"embed"
	"fmt"
	"github.com/ad/domru/cmd/controllers"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/authorizedhttp"
	"github.com/ad/domru/pkg/domru"
	"github.com/ad/domru/pkg/domru/constants"
	"github.com/ad/domru/pkg/logging"
	"github.com/ad/domru/pkg/reverse_proxy"
	"github.com/ad/domru/pkg/token_management"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
)

//go:embed templates/*
var templateFs embed.FS

const (
	flagAccessToken     = "token"
	flagRefreshToken    = "refresh"
	flagLogin           = "login"
	flagPort            = "port"
	flagCredentialsFile = "credentials"
	flagOperatorId      = "operator"
	flagLogLevel        = "log-level"
)

func initFlags() {
	pflag.String(flagAccessToken, "", "dom.ru token")
	pflag.String(flagRefreshToken, "", "dom.ru refresh token")
	pflag.Int(flagLogin, 0, "dom.ru login or phone (i.e: 71231234567)")
	pflag.Int(flagPort, 18000, "listen port")
	pflag.Int(flagOperatorId, 0, "operator id")
	pflag.String(flagCredentialsFile, "accounts.json", "credentials file path (i.e: /usr/domofon/credentials.json")
	pflag.String(flagLogLevel, "info", "log level")
	pflag.Parse()

	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		log.Fatalf("Unable to bind flags: %v", err)
	}

	viper.SetEnvPrefix("domru")
	viper.AutomaticEnv()
}

func initLogger() *slog.Logger {
	logLevel := logging.ParseLogLevel(viper.GetString(flagLogLevel))
	defaultHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel, AddSource: true})
	return slog.New(logging.NewSanitizingLoggerHandler(defaultHandler))
}

func main() {
	initFlags()

	logger := initLogger()

	if viper.GetInt(flagOperatorId) == 0 {
		logger.Error("Operator id is not set")
		pflag.Usage()
		os.Exit(1)
	}

	listenAddr := fmt.Sprintf(":%d", viper.GetInt(flagPort))
	operatorId := viper.GetInt(flagOperatorId)
	credentialsFile := viper.GetString(flagCredentialsFile)

	retryableClient := retryablehttp.NewClient()
	retryableClient.RetryMax = 5

	credentialsStore := auth.NewFileCredentialsStore(credentialsFile)
	tokenProvider := token_management.NewValidTokenProvider(credentialsStore)
	tokenProvider.Logger = logger
	authClient := authorizedhttp.NewClient(
		operatorId,
		tokenProvider,
		tokenProvider,
	)
	authClient.DefaultClient = retryableClient.StandardClient()
	authClient.Logger = logger

	domruAPI := domru.NewDomruAPI(authClient)
	domruAPI.Logger = logger
	handlers := controllers.NewHandlers(templateFs, credentialsStore, domruAPI)
	handlers.Logger = logger

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
			logger.Debug("Proxying request to %s", r.URL)
			proxyHandler(w, r)
		} else {
			logger.Debug("Redirecting to /pages/home.html")
			http.Redirect(w, r, "/pages/home.html", http.StatusMovedPermanently)
		}
	})

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
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}
