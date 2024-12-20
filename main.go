package main

import (
	"embed"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/moleus/domru/cmd/controllers"
	"github.com/moleus/domru/pkg/auth"
	"github.com/moleus/domru/pkg/authorizedhttp"
	"github.com/moleus/domru/pkg/domru"
	"github.com/moleus/domru/pkg/domru/constants"
	"github.com/moleus/domru/pkg/domru/sanitizing_utils"
	"github.com/moleus/domru/pkg/logging"
	"github.com/moleus/domru/pkg/reverseproxy"
	"github.com/moleus/domru/pkg/tokenmanagement"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

//go:embed templates/*
var templateFs embed.FS

const (
	flagPort            = "port"
	flagRefreshToken    = "refresh-token"
	flagOperatorID      = "operator-id"
	flagCredentialsFile = "credentials"
	flagLogLevel        = "log-level"
)

func initFlags() {
	pflag.Int(flagPort, 18000, "listen port")
	pflag.String(flagCredentialsFile, "/share/domofon/accounts.json", "credentials file path (i.e: /usr/domofon/credentials.json")
	pflag.String(flagLogLevel, "info", "log level")
	pflag.String(flagRefreshToken, "", "refresh token")
	pflag.Int(flagOperatorID, 0, "operator id")
	pflag.Parse()

	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		log.Fatalf("Unable to bind flags: %v", err)
	}

	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
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

	authProvider := tokenmanagement.NewValidTokenProvider(credentialsStore)
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

	proxy := reverseproxy.NewReverseProxy(upstream)
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

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      nil,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  50 * time.Second,
	}

	err = server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func overrideCredentialsWithFlags(credentialsStore *auth.FileCredentialsStore, logger *slog.Logger) {
	sanitizedToken := sanitizing_utils.KeepFirstNCharacters(viper.GetString(flagRefreshToken), 7)
	logger.With("refreshToken", sanitizedToken).With("operator-id", viper.GetInt(flagOperatorID)).Debug("Checking flags")
	if viper.GetString(flagRefreshToken) != "" && viper.GetInt(flagOperatorID) != 0 {
		logger.Info("Overriding credentials with flags")
		credentials := auth.Credentials{
			AccessToken:  "",
			RefreshToken: viper.GetString(flagRefreshToken),
			OperatorID:   viper.GetInt(flagOperatorID),
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
