package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/moleus/domru/pkg/callcontrol"
	"github.com/moleus/domru/pkg/domru"
	"github.com/moleus/domru/pkg/sipclient"
	"github.com/moleus/domru/pkg/telegram"
	"github.com/moleus/domru/pkg/webhook"
	"github.com/spf13/viper"
)

type integrations struct {
	sip                     *sipclient.Client
	telegram                *telegram.Bot
	webhook                 *webhook.Sender
	controller              *callcontrol.Controller
	place, control          int
	sipError, telegramError string
	diagnosticToken         string
}

func startIntegrations(ctx context.Context, api *domru.APIWrapper, credentials string, logger *slog.Logger) *integrations {
	viper.SetDefault("sip-port", 5060)
	viper.SetDefault("sip-end-mode", "off")
	viper.SetDefault("sip-rtp-first", 20000)
	viper.SetDefault("sip-rtp-last", 20100)
	x := &integrations{place: viper.GetInt("sip-place-id"), control: viper.GetInt("sip-access-control-id")}
	enabled := viper.GetBool("sip-enabled")
	token := viper.GetString("telegram-bot-token")
	chat := viper.GetString("telegram-chat-id")
	if !enabled && token == "" && chat == "" {
		return x
	}
	if x.place <= 0 || x.control <= 0 {
		x.sipError = "Set DOMRU_SIP_PLACE_ID and DOMRU_SIP_ACCESS_CONTROL_ID"
		logger.Error(x.sipError)
		return x
	}
	x.controller = &callcontrol.Controller{Mode: "off", OpenDoor: func(ctx context.Context) error { return api.OpenIntercom(ctx, x.place, x.control) }}
	if enabled {
		id, err := sipclient.InstallationID(filepath.Join(filepath.Dir(credentials), "sip-installation-id"))
		if err == nil {
			x.sip, err = sipclient.New(sipclient.Config{IP: viper.GetString("sip-ip"), Port: viper.GetInt("sip-port"), Installation: id, RTPFirst: viper.GetInt("sip-rtp-first"), RTPLast: viper.GetInt("sip-rtp-last")})
		}
		if err != nil {
			x.sipError = "Cannot initialize SIP: check local IP, ports and writable state directory"
			logger.Error(x.sipError)
		} else {
			x.sip.Fetch = func(ctx context.Context) (domru.SIPCredentials, string, error) {
				return api.SIPDevice(ctx, x.place, x.control, id)
			}
			x.controller.Calls = x.sip
			mode := viper.GetString("sip-end-mode")
			if mode == "off" || mode == "reject" || mode == "answer-bye" {
				x.controller.Mode = mode
			} else {
				x.sipError = "Invalid DOMRU_SIP_END_MODE; using off"
			}
			if viper.GetBool("sip-diagnostics") {
				x.diagnosticToken = viper.GetString("sip-diagnostics-token")
				if len(x.diagnosticToken) < 24 {
					x.diagnosticToken = ""
					x.sipError = "Diagnostics require DOMRU_SIP_DIAGNOSTICS_TOKEN of at least 24 characters"
				}
			}
		}
	}
	if token != "" || chat != "" {
		chatID, err := strconv.ParseInt(chat, 10, 64)
		if err == nil && token != "" && chatID != 0 {
			x.telegram, err = telegram.New(telegram.Config{Token: token, ChatID: chatID, PlaceID: x.place, ControlID: x.control, StateFile: filepath.Join(filepath.Dir(credentials), "telegram-state.json")})
		} else {
			x.telegramError = "Set both Telegram token and numeric chat ID"
		}
		if err != nil {
			x.telegramError = "Cannot initialize Telegram: check configuration and state file"
		}
		if x.telegram != nil {
			x.telegram.Snapshot = func(ctx context.Context) ([]byte, error) { return api.IntercomSnapshot(ctx, x.place, x.control) }
			x.telegram.Open = x.controller.Open
			go x.telegram.Run(ctx)
		}
	}
	if webhookURL := viper.GetString("webhook-url"); webhookURL != "" {
		u, err := url.Parse(webhookURL)
		if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			x.webhook = webhook.New(webhookURL)
			go x.webhook.Run(ctx)
		} else {
			logger.Error("Invalid DOMRU_WEBHOOK_URL")
		}
	}
	if x.sip != nil {
		x.sip.Ring = func(e callcontrol.Event) {
			if x.telegram != nil {
				x.telegram.Enqueue(e)
			}
			if x.webhook != nil {
				x.webhook.Enqueue(e)
			}
		}
		go x.sip.Run(ctx)
	}
	return x
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (x *integrations) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/integrations/state", func(w http.ResponseWriter, r *http.Request) {
		sipStatus := sipclient.Status{State: "off", Calls: []sipclient.CallStatus{}}
		if x.sip != nil {
			sipStatus = x.sip.Status()
		}
		if x.sipError != "" {
			sipStatus.Error = x.sipError
			if x.sip == nil {
				sipStatus.State = "error"
			}
		}
		tgStatus := telegram.Status{State: "off"}
		if x.telegram != nil {
			tgStatus = x.telegram.Status()
		}
		if x.telegramError != "" {
			tgStatus = telegram.Status{State: "error", Error: x.telegramError}
		}
		webhookError := ""
		if x.webhook != nil {
			webhookError = x.webhook.Error()
		}
		mode := "off"
		if x.controller != nil {
			mode = x.controller.Mode
		}
		writeJSON(w, 200, map[string]any{"sip": sipStatus, "telegram": tgStatus, "webhook_error": webhookError, "end_mode": mode})
	})
	mux.HandleFunc("POST /api/places/{placeId}/accesscontrols/{accessControlId}/open-and-end-call", func(w http.ResponseWriter, r *http.Request) {
		place, e1 := strconv.Atoi(r.PathValue("placeId"))
		control, e2 := strconv.Atoi(r.PathValue("accessControlId"))
		if x.controller == nil || e1 != nil || e2 != nil || place != x.place || control != x.control {
			http.Error(w, "Intercom is not configured", 404)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "Cross-origin opening is not allowed", 403)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		result := x.controller.Open(ctx, nil)
		status := 200
		if result.Opening == "busy" {
			status = 409
		} else if result.Opening != "accepted" {
			status = 502
		}
		writeJSON(w, status, result)
	})
	mux.HandleFunc("POST /api/sip/calls/{callId}/{action}", func(w http.ResponseWriter, r *http.Request) {
		if x.sip == nil || x.diagnosticToken == "" {
			http.NotFound(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+x.diagnosticToken)) != 1 {
			http.Error(w, "Diagnostic token required", 401)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		var err error
		switch r.PathValue("action") {
		case "reject":
			err = x.sip.Reject(r.PathValue("callId"))
		case "answer-bye":
			err = x.sip.Answer(ctx, r.PathValue("callId"))
			if err == nil {
				err = x.sip.Bye(ctx, r.PathValue("callId"))
			}
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			writeJSON(w, 409, map[string]string{"result": "failed", "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"result": "ended"})
	})
}
func sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}
