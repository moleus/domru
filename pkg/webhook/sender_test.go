package webhook

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/moleus/domru/pkg/callcontrol"
	"github.com/stretchr/testify/require"
)

func TestOneWebhookPerCallWithRetries(t *testing.T) {
	var mu sync.Mutex
	var keys, bodies []string
	codes := []int{500, 200, 400}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		bodies = append(bodies, string(body))
		code := codes[0]
		if len(codes) > 1 {
			codes = codes[1:]
		}
		mu.Unlock()
		w.WriteHeader(code)
	}))
	defer srv.Close()
	s := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	s.Enqueue(callcontrol.Event{ID: "call-1", Time: time.Now(), Name: "door"})
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(keys) == 2 }, 5*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return s.Error() == "" }, time.Second, 10*time.Millisecond)
	// 5xx is retried with the same Idempotency-Key and the fixed Ringing body.
	mu.Lock()
	require.Equal(t, []string{"call-1", "call-1"}, keys)
	require.Equal(t, `{"event":"Ringing"}`, bodies[0])
	mu.Unlock()
	// 4xx is final: one attempt, error exposed without the URL.
	s.Enqueue(callcontrol.Event{ID: "call-2", Time: time.Now(), Name: "door"})
	require.Eventually(t, func() bool { return s.Error() == "Webhook HTTP 400" }, 3*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	require.Len(t, keys, 3)
	mu.Unlock()
}

func TestReceiverTimeoutDoesNotBlockQueue(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	s := New(srv.URL)
	s.Client.Timeout = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	s.Enqueue(callcontrol.Event{ID: "call-1"})
	require.Eventually(t, func() bool { return s.Error() == "Webhook network error" }, 2*time.Second, 10*time.Millisecond)
	for i := 0; i < 40; i++ {
		s.Enqueue(callcontrol.Event{ID: "flood"})
	}
	require.Equal(t, "Webhook queue full", s.Error())
}
