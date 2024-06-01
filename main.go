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
	flagPort            = "port"
	flagRefreshToken    = "refresh-token"
	flagOperatorId      = "operator-id"
	flagCredentialsFile = "credentials"
	flagLogLevel        = "log-level"
)

func initFlags() {
	pflag.Int(flagPort, 18000, "listen port")
	pflag.String(flagCredentialsFile, "accounts.json", "credentials file path (i.e: /usr/domofon/credentials.json")
	pflag.String(flagLogLevel, "info", "log level")
	pflag.String(flagRefreshToken, "", "refresh token")
	pflag.Int(flagOperatorId, 0, "operator id")
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

	listenAddr := fmt.Sprintf(":%d", viper.GetInt(flagPort))
	credentialsFile := viper.GetString(flagCredentialsFile)

	retryableClient := retryablehttp.NewClient()
	retryableClient.RetryMax = 5

	credentialsStore := auth.NewFileCredentialsStore(credentialsFile)

	overrideCredentialsWithFlags(credentialsStore, logger)

	authProvider := token_management.NewValidTokenProvider(credentialsStore)
	authProvider.Logger = logger
	authClient := authorizedhttp.NewClient(
		authProvider,
		authProvider,
		authProvider,
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
			logger.With("url", r.URL.String()).Debug("proxying request")
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

func overrideCredentialsWithFlags(credentialsStore *auth.FileCredentialsStore, logger *slog.Logger) {
	if viper.GetString(flagRefreshToken) != "" && viper.GetInt(flagOperatorId) != 0 {
		credentials := auth.Credentials{
			AccessToken:  "",
			RefreshToken: viper.GetString(flagRefreshToken),
			OperatorID:   viper.GetInt(flagOperatorId),
		}
		err := credentialsStore.SaveCredentials(credentials)
		if err != nil {
			logger.With("err", err.Error()).Error("Unable to save credentials")
		}
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
