package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moleus/domru/pkg/callcontrol"
	"github.com/stretchr/testify/require"
)

// fakeTelegram scripts getUpdates and records every other Bot API call.
type fakeTelegram struct {
	t       *testing.T
	mu      sync.Mutex
	calls   []string            // method names in order
	bodies  map[string][]string // method -> raw bodies (JSON or multipart)
	updates chan []byte         // one getUpdates response per element
	fail    map[string][]int    // method -> queued error codes
}

func newFake(t *testing.T) (*fakeTelegram, *httptest.Server) {
	f := &fakeTelegram{t: t, bodies: map[string][]string{}, updates: make(chan []byte, 10), fail: map[string][]int{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}
func (f *fakeTelegram) serve(w http.ResponseWriter, r *http.Request) {
	require.True(f.t, strings.HasPrefix(r.URL.Path, "/botTOKEN/"), r.URL.Path)
	method := strings.TrimPrefix(r.URL.Path, "/botTOKEN/")
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, method)
	f.bodies[method] = append(f.bodies[method], string(body))
	if codes := f.fail[method]; len(codes) > 0 {
		code := codes[0]
		f.fail[method] = codes[1:]
		f.mu.Unlock()
		w.WriteHeader(code)
		fmt.Fprintf(w, `{"ok":false,"error_code":%d,"parameters":{"retry_after":0}}`, code)
		return
	}
	f.mu.Unlock()
	switch method {
	case "getMe":
		io.WriteString(w, `{"ok":true,"result":{"id":42}}`)
	case "getWebhookInfo":
		io.WriteString(w, `{"ok":true,"result":{"url":""}}`)
	case "getUpdates":
		select {
		case u := <-f.updates:
			w.Write(u)
		case <-time.After(100 * time.Millisecond):
			io.WriteString(w, `{"ok":true,"result":[]}`)
		}
	case "sendPhoto", "sendMessage":
		io.WriteString(w, `{"ok":true,"result":{"message_id":101}}`)
	default:
		io.WriteString(w, `{"ok":true,"result":true}`)
	}
}
func (f *fakeTelegram) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies[method])
}
func (f *fakeTelegram) last(method string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.bodies[method]
	if len(b) == 0 {
		return ""
	}
	return b[len(b)-1]
}
func (f *fakeTelegram) waitFor(method string, n int) {
	require.Eventually(f.t, func() bool { return f.count(method) >= n }, 3*time.Second, 10*time.Millisecond, method)
}

type env struct {
	f     *fakeTelegram
	bot   *Bot
	state string
	opens []string // expected call IDs passed to Open
	mu    sync.Mutex
}

func start(t *testing.T, f *fakeTelegram, srv *httptest.Server, state string, open func(context.Context, *string) callcontrol.Result) *env {
	bot, err := New(Config{Token: "TOKEN", ChatID: -100, PlaceID: 1, ControlID: 2, StateFile: state})
	require.NoError(t, err)
	bot.BaseURL = srv.URL
	e := &env{f: f, bot: bot, state: state}
	bot.Snapshot = func(context.Context) ([]byte, error) { return []byte("\xff\xd8\xffjpeg"), nil }
	bot.Open = func(ctx context.Context, id *string) callcontrol.Result {
		e.mu.Lock()
		e.opens = append(e.opens, *id)
		e.mu.Unlock()
		if open != nil {
			return open(ctx, id)
		}
		return callcontrol.Result{Opening: "accepted", Call: "off"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { bot.Run(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("bot shutdown stuck")
		}
	})
	require.Eventually(t, func() bool { return bot.Status().State == "ready" }, 3*time.Second, 10*time.Millisecond)
	return e
}
func (e *env) opened() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string{}, e.opens...)
}

// buttonID extracts the opaque id from the last sent keyboard.
func buttonID(t *testing.T, body string) string {
	i := strings.Index(body, "open:")
	require.GreaterOrEqual(t, i, 0, body)
	return body[i+5 : i+5+36]
}
func callbackUpdate(updateID int64, cbID, data string, chat int64, chatType string, from int64, msg int64, photo bool) []byte {
	m := map[string]any{"message_id": msg, "chat": map[string]any{"id": chat, "type": chatType}, "caption": "Входящий звонок: door"}
	if photo {
		m["photo"] = []map[string]any{{"file_id": "x"}}
	} else {
		m["text"] = "Входящий звонок: door"
	}
	u := map[string]any{"ok": true, "result": []map[string]any{{"update_id": updateID, "callback_query": map[string]any{"id": cbID, "data": data, "from": map[string]any{"id": from}, "message": m}}}}
	b, _ := json.Marshal(u)
	return b
}

func TestOneNotificationWithPhotoThenTextFallback(t *testing.T) {
	f, srv := newFake(t)
	e := start(t, f, srv, filepath.Join(t.TempDir(), "tg.json"), nil)
	e.bot.Enqueue(callcontrol.Event{ID: "call-1", Time: time.Now(), Name: "door"})
	f.waitFor("sendPhoto", 1)
	body := f.last("sendPhoto")
	require.Contains(t, body, "Звонок в домофон: door")
	require.Contains(t, body, `"text":"Открыть дверь"`)
	require.Contains(t, body, "\xff\xd8\xffjpeg")
	require.NotContains(t, body, "TOKEN\"")
	// Snapshot failure produces a text message with the same kind of button; no stale photo.
	e.bot.Snapshot = func(context.Context) ([]byte, error) { return nil, errors.New("timeout") }
	e.bot.Enqueue(callcontrol.Event{ID: "call-2", Time: time.Now(), Name: "door"})
	f.waitFor("sendMessage", 1)
	require.Contains(t, f.last("sendMessage"), "Снимок недоступен")
	require.Contains(t, f.last("sendMessage"), "open:")
	require.Equal(t, 1, f.count("sendPhoto"))
	// Bindings are persisted with the message id and original call id.
	data, err := os.ReadFile(e.state)
	require.NoError(t, err)
	var st state
	require.NoError(t, json.Unmarshal(data, &st))
	require.Len(t, st.Notes, 2)
	for _, n := range st.Notes {
		require.Len(t, n.Messages, 1)
		require.Contains(t, []string{"call-1", "call-2"}, n.Call)
	}
}

func TestRetryAfterOn429(t *testing.T) {
	f, srv := newFake(t)
	f.fail["sendPhoto"] = []int{429}
	e := start(t, f, srv, filepath.Join(t.TempDir(), "tg.json"), nil)
	e.bot.Enqueue(callcontrol.Event{ID: "call-1", Time: time.Now(), Name: "door"})
	f.waitFor("sendPhoto", 2)
	require.Eventually(t, func() bool { return e.bot.Status().State == "ready" }, 3*time.Second, 10*time.Millisecond)
	// 4xx other than 429 is final: exactly one attempt.
	f.fail["sendPhoto"] = []int{400}
	e.bot.Enqueue(callcontrol.Event{ID: "call-2", Time: time.Now(), Name: "door"})
	f.waitFor("sendPhoto", 3)
	time.Sleep(150 * time.Millisecond)
	require.Equal(t, 3, f.count("sendPhoto"))
}

func TestCallbackValidationAndDuplicates(t *testing.T) {
	f, srv := newFake(t)
	e := start(t, f, srv, filepath.Join(t.TempDir(), "tg.json"), nil)
	e.bot.Enqueue(callcontrol.Event{ID: "call-1", Time: time.Now(), Name: "door"})
	f.waitFor("sendPhoto", 1)
	id := buttonID(t, f.last("sendPhoto"))
	const msg = 101
	// Forged button id, foreign chat, forwarded copy (unknown message id): refused, no Open.
	f.updates <- callbackUpdate(1, "cb-forged", "open:00000000-0000-0000-0000-000000000000", -100, "supergroup", 7, msg, true)
	f.updates <- callbackUpdate(2, "cb-foreign", "open:"+id, -200, "supergroup", 7, msg, true)
	f.updates <- callbackUpdate(3, "cb-forwarded", "open:"+id, -100, "supergroup", 7, 999, true)
	f.waitFor("answerCallbackQuery", 3)
	require.Empty(t, e.opened())
	// Answers only stop the button spinner: no toast text, ever.
	require.NotContains(t, f.last("answerCallbackQuery"), `"text"`)
	// Valid group member click opens once; success is silent.
	f.updates <- callbackUpdate(4, "cb-ok", "open:"+id, -100, "supergroup", 7, msg, true)
	f.waitFor("answerCallbackQuery", 4)
	require.Eventually(t, func() bool { return len(e.opened()) == 1 }, time.Second, 10*time.Millisecond)
	// Duplicate delivery of the same callback: answered, not re-opened.
	f.updates <- callbackUpdate(4, "cb-ok", "open:"+id, -100, "supergroup", 7, msg, true)
	f.updates <- callbackUpdate(5, "cb-ok", "open:"+id, -100, "supergroup", 8, msg, true)
	f.waitFor("answerCallbackQuery", 5)
	require.Equal(t, []string{"call-1"}, e.opened())
	// The button has no expiry: a new click repeats the opening with the original call id.
	f.updates <- callbackUpdate(6, "cb-again", "open:"+id, -100, "supergroup", 7, msg, true)
	f.waitFor("answerCallbackQuery", 6)
	require.Eventually(t, func() bool { return len(e.opened()) == 2 }, time.Second, 10*time.Millisecond)
	require.Equal(t, []string{"call-1", "call-1"}, e.opened())
	// The notification is never edited and successes produce no extra messages.
	require.Zero(t, f.count("editMessageCaption"))
	require.Zero(t, f.count("editMessageText"))
	require.Zero(t, f.count("sendMessage"))
	require.NotContains(t, f.last("answerCallbackQuery"), `"text"`)
}

func TestFailedOpeningGetsOneWarningReply(t *testing.T) {
	f, srv := newFake(t)
	e := start(t, f, srv, filepath.Join(t.TempDir(), "tg.json"), func(context.Context, *string) callcontrol.Result {
		return callcontrol.Result{Opening: "failed", Call: "failed"}
	})
	e.bot.Enqueue(callcontrol.Event{ID: "call-1", Time: time.Now(), Name: "door"})
	f.waitFor("sendPhoto", 1)
	id := buttonID(t, f.last("sendPhoto"))
	f.updates <- callbackUpdate(1, "cb-fail", "open:"+id, -100, "supergroup", 7, 101, true)
	f.waitFor("sendMessage", 1)
	warn := f.last("sendMessage")
	require.Contains(t, warn, "⚠️")
	require.Contains(t, warn, "Открытие не выполнено")
	require.Contains(t, warn, `"reply_to_message_id":101`)
	require.Zero(t, f.count("editMessageCaption"))
}

func TestSimultaneousClicksNotQueued(t *testing.T) {
	f, srv := newFake(t)
	release := make(chan struct{})
	ctl := &callcontrol.Controller{Mode: "off", OpenDoor: func(context.Context) error { <-release; return nil }}
	e := start(t, f, srv, filepath.Join(t.TempDir(), "tg.json"), ctl.Open)
	e.bot.Enqueue(callcontrol.Event{ID: "call-1", Time: time.Now(), Name: "door"})
	f.waitFor("sendPhoto", 1)
	id := buttonID(t, f.last("sendPhoto"))
	f.updates <- callbackUpdate(1, "cb-a", "open:"+id, -100, "group", 7, 101, true)
	f.waitFor("answerCallbackQuery", 1)
	require.Eventually(t, func() bool { return len(e.opened()) == 1 }, time.Second, 10*time.Millisecond)
	f.updates <- callbackUpdate(2, "cb-b", "open:"+id, -100, "group", 8, 101, true)
	f.waitFor("answerCallbackQuery", 2)
	close(release)
	// Both clicks reach the controller; the second gets busy, is not queued and stays silent.
	require.Eventually(t, func() bool {
		e.bot.mu.Lock()
		defer e.bot.mu.Unlock()
		return len(e.bot.disk.Done) == 2
	}, 3*time.Second, 10*time.Millisecond)
	require.Equal(t, []string{"call-1", "call-1"}, e.opened())
	e.bot.mu.Lock()
	require.Equal(t, "busy", e.bot.disk.Done["cb-b"].Opening)
	e.bot.mu.Unlock()
	require.Zero(t, f.count("sendMessage"))
	require.Zero(t, f.count("editMessageCaption"))
}

func TestPrivateChatRequiresOwner(t *testing.T) {
	f, srv := newFake(t)
	state := filepath.Join(t.TempDir(), "tg.json")
	bot, err := New(Config{Token: "TOKEN", ChatID: 555, PlaceID: 1, ControlID: 2, StateFile: state})
	require.NoError(t, err)
	bot.BaseURL = srv.URL
	var opens atomic.Int32
	bot.Snapshot = func(context.Context) ([]byte, error) { return nil, errors.New("no") }
	bot.Open = func(context.Context, *string) callcontrol.Result {
		opens.Add(1)
		return callcontrol.Result{Opening: "accepted", Call: "off"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bot.Run(ctx)
	require.Eventually(t, func() bool { return bot.Status().State == "ready" }, 3*time.Second, 10*time.Millisecond)
	bot.Enqueue(callcontrol.Event{ID: "call-1", Time: time.Now(), Name: "door"})
	f.waitFor("sendMessage", 1)
	id := buttonID(t, f.last("sendMessage"))
	f.updates <- callbackUpdate(1, "cb-stranger", "open:"+id, 555, "private", 556, 101, false)
	f.waitFor("answerCallbackQuery", 1)
	require.Zero(t, opens.Load())
	f.updates <- callbackUpdate(2, "cb-owner", "open:"+id, 555, "private", 555, 101, false)
	f.waitFor("answerCallbackQuery", 2)
	require.Eventually(t, func() bool { return opens.Load() == 1 }, time.Second, 10*time.Millisecond)
	require.Zero(t, f.count("editMessageText"))
}

func TestRestartKeepsButtonsAndDoesNotReplayOpening(t *testing.T) {
	f, srv := newFake(t)
	file := filepath.Join(t.TempDir(), "tg.json")
	e := start(t, f, srv, file, nil)
	e.bot.Enqueue(callcontrol.Event{ID: "call-1", Time: time.Now(), Name: "door"})
	f.waitFor("sendPhoto", 1)
	id := buttonID(t, f.last("sendPhoto"))
	// Simulate a crash in the middle of an opening: the callback is marked processing on disk.
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	var st state
	require.NoError(t, json.Unmarshal(data, &st))
	st.Done["cb-crashed"] = callcontrol.Result{Opening: "processing", Call: "not_attempted"}
	st.Offset = 10
	data, _ = json.Marshal(st)
	require.NoError(t, os.WriteFile(file, data, 0600))

	f2, srv2 := newFake(t)
	e2 := start(t, f2, srv2, file, nil)
	info, err := os.Stat(file)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	// Redelivered crashed callback: reported as unknown, never re-opened.
	f2.updates <- callbackUpdate(10, "cb-crashed", "open:"+id, -100, "supergroup", 7, 101, true)
	f2.waitFor("answerCallbackQuery", 1)
	require.Empty(t, e2.opened())
	// Old update ids below the stored offset are ignored.
	f2.updates <- callbackUpdate(3, "cb-old", "open:"+id, -100, "supergroup", 7, 101, true)
	// The old button still works after restart with unchanged chat and intercom.
	f2.updates <- callbackUpdate(11, "cb-after-restart", "open:"+id, -100, "supergroup", 7, 101, true)
	f2.waitFor("answerCallbackQuery", 2)
	require.Eventually(t, func() bool { return len(e2.opened()) == 1 }, time.Second, 10*time.Millisecond)
	require.Equal(t, []string{"call-1"}, e2.opened())
	require.Equal(t, 2, f2.count("answerCallbackQuery"))
}

func TestChangedIntercomInvalidatesOldButtons(t *testing.T) {
	f, srv := newFake(t)
	state := filepath.Join(t.TempDir(), "tg.json")
	e := start(t, f, srv, state, nil)
	e.bot.Enqueue(callcontrol.Event{ID: "call-1", Time: time.Now(), Name: "door"})
	f.waitFor("sendPhoto", 1)
	id := buttonID(t, f.last("sendPhoto"))
	f2, srv2 := newFake(t)
	bot, err := New(Config{Token: "TOKEN", ChatID: -100, PlaceID: 1, ControlID: 3, StateFile: state})
	require.NoError(t, err)
	bot.BaseURL = srv2.URL
	opens := 0
	bot.Open = func(context.Context, *string) callcontrol.Result { opens++; return callcontrol.Result{} }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bot.Run(ctx)
	require.Eventually(t, func() bool { return bot.Status().State == "ready" }, 3*time.Second, 10*time.Millisecond)
	f2.updates <- callbackUpdate(1, "cb-old-door", "open:"+id, -100, "supergroup", 7, 101, true)
	f2.waitFor("answerCallbackQuery", 1)
	require.Zero(t, opens)
	require.Zero(t, f2.count("sendMessage"))
}
