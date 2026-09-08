// Package telegram delivers intercom events and handles durable inline buttons.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/moleus/domru/pkg/atomicfile"
	"github.com/moleus/domru/pkg/callcontrol"
)

type Config struct {
	Token              string
	ChatID             int64
	PlaceID, ControlID int
	StateFile          string
}
type notification struct {
	Call           string `json:"call"`
	Chat           int64  `json:"chat"`
	Place, Control int
	Messages       []int64 `json:"messages"`
}
type state struct {
	Offset int64                         `json:"offset"`
	BotID  int64                         `json:"bot_id"`
	Notes  map[string]notification       `json:"notifications"`
	Done   map[string]callcontrol.Result `json:"callbacks"`
}
type Status struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}
type Bot struct {
	cfg      Config
	Client   *http.Client
	BaseURL  string
	Snapshot func(context.Context) ([]byte, error)
	Open     func(context.Context, *string) callcontrol.Result
	mu       sync.Mutex
	disk     state
	status   Status
	queue    chan callcontrol.Event
}

type apiError struct {
	code  int
	retry int
}

func (e apiError) Error() string { return fmt.Sprintf("Telegram API error %d", e.code) }
func New(cfg Config) (*Bot, error) {
	if cfg.Token == "" || cfg.ChatID == 0 {
		return nil, errors.New("Telegram token and numeric chat ID required")
	}
	b := &Bot{cfg: cfg, BaseURL: "https://api.telegram.org", Client: &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, queue: make(chan callcontrol.Event, 32), status: Status{State: "starting"}, disk: state{Notes: map[string]notification{}, Done: map[string]callcontrol.Result{}}}
	data, err := os.ReadFile(cfg.StateFile)
	if err == nil {
		if json.Unmarshal(data, &b.disk) != nil {
			return nil, errors.New("invalid Telegram state file")
		}
	} else if !os.IsNotExist(err) {
		return nil, errors.New("cannot read Telegram state")
	}
	if b.disk.Notes == nil {
		b.disk.Notes = map[string]notification{}
	}
	if b.disk.Done == nil {
		b.disk.Done = map[string]callcontrol.Result{}
	}
	for id, r := range b.disk.Done {
		if r.Opening == "processing" {
			b.disk.Done[id] = callcontrol.Result{Opening: "unknown", Call: "not_attempted"}
		}
	}
	if err = b.save(); err != nil {
		return nil, errors.New("cannot save Telegram state")
	}
	return b, nil
}
func (b *Bot) save() error           { return atomicfile.WriteJSON(b.cfg.StateFile, b.disk) }
func (b *Bot) Status() Status        { b.mu.Lock(); defer b.mu.Unlock(); return b.status }
func (b *Bot) set(state, msg string) { b.mu.Lock(); b.status = Status{state, msg}; b.mu.Unlock() }
func (b *Bot) Enqueue(e callcontrol.Event) {
	select {
	case b.queue <- e:
	default:
		b.set("error", "Telegram notification queue full")
	}
}

// api never exposes the token-bearing URL or Telegram response body in errors.
func (b *Bot) api(ctx context.Context, method string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.BaseURL+"/bot"+b.cfg.Token+"/"+method, body)
	if err != nil {
		return errors.New("cannot create Telegram request")
	}
	req.Header.Set("Content-Type", contentType)
	res, err := b.Client.Do(req)
	if err != nil {
		return errors.New("Telegram network request failed")
	}
	defer res.Body.Close()
	var envelope struct {
		OK         bool            `json:"ok"`
		Code       int             `json:"error_code"`
		Result     json.RawMessage `json:"result"`
		Parameters struct {
			Retry int `json:"retry_after"`
		} `json:"parameters"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&envelope) != nil {
		return errors.New("invalid Telegram response")
	}
	if !envelope.OK || res.StatusCode != 200 {
		code := envelope.Code
		if code == 0 {
			code = res.StatusCode
		}
		return apiError{code, envelope.Parameters.Retry}
	}
	if out != nil && json.Unmarshal(envelope.Result, out) != nil {
		return errors.New("invalid Telegram result")
	}
	return nil
}
func (b *Bot) json(ctx context.Context, method string, payload any, out any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.New("cannot encode Telegram request")
	}
	return b.api(ctx, method, bytes.NewReader(data), "application/json", out)
}
func wait(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (b *Bot) Run(ctx context.Context) {
	for ctx.Err() == nil {
		var me struct {
			ID int64 `json:"id"`
		}
		if err := b.json(ctx, "getMe", struct{}{}, &me); err != nil {
			b.set("error", err.Error())
			if !wait(ctx, 15*time.Second) {
				return
			}
			continue
		}
		var info struct {
			URL string `json:"url"`
		}
		if err := b.json(ctx, "getWebhookInfo", struct{}{}, &info); err != nil {
			b.set("error", err.Error())
			if !wait(ctx, 15*time.Second) {
				return
			}
			continue
		}
		if info.URL != "" {
			b.set("error", "Telegram bot already has a webhook; use a dedicated bot")
			if !wait(ctx, 30*time.Second) {
				return
			}
			continue
		}
		b.mu.Lock()
		if b.disk.BotID != 0 && b.disk.BotID != me.ID {
			b.disk = state{Notes: map[string]notification{}, Done: map[string]callcontrol.Result{}}
		}
		b.disk.BotID = me.ID
		err := b.save()
		b.mu.Unlock()
		if err != nil {
			b.set("error", "Cannot save Telegram state")
			return
		}
		break
	}
	if ctx.Err() != nil {
		return
	}
	b.set("ready", "")
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for {
			select {
			case e := <-b.queue:
				b.notify(ctx, e)
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() { <-workerDone; b.set("off", "") }()
	for ctx.Err() == nil {
		b.mu.Lock()
		offset := b.disk.Offset
		b.mu.Unlock()
		var updates []update
		err := b.json(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 25, "allowed_updates": []string{"callback_query"}}, &updates)
		if err != nil {
			b.set("error", err.Error())
			if !wait(ctx, 5*time.Second) {
				return
			}
			continue
		}
		for _, u := range updates {
			if ctx.Err() != nil {
				return
			}
			b.handle(ctx, u)
		}
	}
}
func (b *Bot) notify(ctx context.Context, e callcontrol.Event) {
	id := uuid.NewString()
	b.mu.Lock()
	b.disk.Notes[id] = notification{Call: e.ID, Chat: b.cfg.ChatID, Place: b.cfg.PlaceID, Control: b.cfg.ControlID}
	err := b.save()
	b.mu.Unlock()
	if err != nil {
		b.set("error", "Cannot persist Telegram button")
		return
	}
	snapshot, cancel := context.WithTimeout(ctx, 3*time.Second)
	photo, photoErr := b.Snapshot(snapshot)
	cancel()
	// ponytail: no timestamp, Telegram shows the message time itself.
	caption := "🔔 Звонок в домофон: " + e.Name
	if photoErr != nil {
		photo = nil
		caption += "\nСнимок недоступен"
	}
	keyboard := map[string]any{"inline_keyboard": [][]map[string]string{{{"text": "Открыть дверь", "callback_data": "open:" + id}}}}
	var message struct {
		ID int64 `json:"message_id"`
	}
	for attempt := 0; attempt < 3; attempt++ {
		if len(photo) == 0 {
			err = b.json(ctx, "sendMessage", map[string]any{"chat_id": b.cfg.ChatID, "text": caption, "reply_markup": keyboard}, &message)
		} else {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			_ = form.WriteField("chat_id", strconv.FormatInt(b.cfg.ChatID, 10))
			_ = form.WriteField("caption", caption)
			markup, _ := json.Marshal(keyboard)
			_ = form.WriteField("reply_markup", string(markup))
			part, _ := form.CreateFormFile("photo", "intercom.jpg")
			_, _ = part.Write(photo)
			_ = form.Close()
			err = b.api(ctx, "sendPhoto", &body, form.FormDataContentType(), &message)
		}
		if err == nil {
			b.mu.Lock()
			n := b.disk.Notes[id]
			n.Messages = append(n.Messages, message.ID)
			b.disk.Notes[id] = n
			err = b.save()
			b.mu.Unlock()
			if err != nil {
				b.set("error", "Cannot persist Telegram message binding")
			} else {
				b.set("ready", "")
			}
			return
		}
		b.set("error", err.Error())
		var ae apiError
		delay := time.Duration(attempt+1) * time.Second
		if errors.As(err, &ae) {
			if ae.code == 429 {
				delay = time.Duration(ae.retry) * time.Second
			} else if ae.code < 500 {
				return
			}
		}
		if !wait(ctx, delay) {
			return
		}
	}
}

type update struct {
	ID       int64     `json:"update_id"`
	Callback *callback `json:"callback_query"`
}
type callback struct {
	ID   string `json:"id"`
	Data string `json:"data"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Message *struct {
		ID   int64 `json:"message_id"`
		Date int64 `json:"date"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		Photo   []json.RawMessage `json:"photo"`
		Text    string            `json:"text"`
		Caption string            `json:"caption"`
	} `json:"message"`
}

// ponytail: an empty answer only stops the button spinner; no toasts, no result text.
func (b *Bot) respond(ctx context.Context, id string) {
	short, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_ = b.json(short, "answerCallbackQuery", map[string]any{"callback_query_id": id}, nil)
}
func (b *Bot) handle(ctx context.Context, u update) {
	b.mu.Lock()
	if u.ID < b.disk.Offset {
		b.mu.Unlock()
		return
	}
	cb := u.Callback
	var n notification
	valid := false
	if cb != nil && cb.Message != nil && len(cb.Data) > 5 && cb.Data[:5] == "open:" {
		n, valid = b.disk.Notes[cb.Data[5:]]
		valid = valid && n.Chat == b.cfg.ChatID && cb.Message.Chat.ID == b.cfg.ChatID && n.Place == b.cfg.PlaceID && n.Control == b.cfg.ControlID
		known := false
		for _, id := range n.Messages {
			if id == cb.Message.ID {
				known = true
			}
		}
		valid = valid && known
		if cb.Message.Chat.Type == "private" {
			valid = valid && cb.From.ID == b.cfg.ChatID
		} else {
			valid = valid && (cb.Message.Chat.Type == "group" || cb.Message.Chat.Type == "supergroup")
		}
	}
	done := false
	if cb != nil {
		_, done = b.disk.Done[cb.ID]
	}
	oldOffset := b.disk.Offset
	b.disk.Offset = u.ID + 1
	if valid && !done {
		b.disk.Done[cb.ID] = callcontrol.Result{Opening: "processing", Call: "not_attempted"}
	}
	if b.save() != nil {
		b.disk.Offset = oldOffset
		if valid && !done {
			delete(b.disk.Done, cb.ID)
		}
		b.mu.Unlock()
		b.set("error", "Cannot persist Telegram callback; opening refused")
		return
	}
	b.mu.Unlock()
	if cb == nil {
		return
	}
	if !valid {
		b.respond(ctx, cb.ID)
		return
	}
	if done {
		b.respond(ctx, cb.ID)
		return
	}
	// Do not let the long poll serialize simultaneous clicks into later opens.
	go func() {
		b.respond(ctx, cb.ID)
		op, cancel := context.WithTimeout(ctx, 20*time.Second)
		result := b.Open(op, &n.Call)
		cancel()
		b.mu.Lock()
		b.disk.Done[cb.ID] = result
		err := b.save()
		b.mu.Unlock()
		if err != nil {
			b.set("error", "Cannot persist opening result")
		}
		// ponytail: success and a busy double click are silent, the message is never
		// edited; only a failure gets a short reply under the notification.
		if result.Opening != "failed" && result.Opening != "unknown" && result.Call != "failed" {
			return
		}
		warn, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		payload := map[string]any{"chat_id": b.cfg.ChatID, "reply_to_message_id": cb.Message.ID, "text": "⚠️ " + result.Text()}
		if err := b.json(warn, "sendMessage", payload, nil); err != nil {
			b.set("error", err.Error())
		}
	}()
}
