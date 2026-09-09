package webhook

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/moleus/domru/pkg/callcontrol"
)

type Sender struct {
	URL       string
	Client    *http.Client
	queue     chan callcontrol.Event
	mu        sync.Mutex
	lastError string
}

func New(url string) *Sender {
	return &Sender{URL: url, Client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, queue: make(chan callcontrol.Event, 32)}
}
func (s *Sender) set(err string) { s.mu.Lock(); s.lastError = err; s.mu.Unlock() }
func (s *Sender) Error() string  { s.mu.Lock(); defer s.mu.Unlock(); return s.lastError }
func (s *Sender) Enqueue(e callcontrol.Event) {
	select {
	case s.queue <- e:
	default:
		s.set("Webhook queue full")
	}
}
func (s *Sender) Run(ctx context.Context) {
	for {
		select {
		case e := <-s.queue:
			s.send(ctx, e)
		case <-ctx.Done():
			return
		}
	}
}
func (s *Sender) send(ctx context.Context, e callcontrol.Event) {
	for i := 0; i < 3; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewBufferString(`{"event":"Ringing"}`))
		if err != nil {
			s.set("Invalid webhook URL")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", e.ID)
		res, err := s.Client.Do(req)
		retry := true
		if err == nil {
			res.Body.Close()
			if res.StatusCode >= 200 && res.StatusCode < 300 {
				s.set("")
				return
			}
			s.set(fmt.Sprintf("Webhook HTTP %d", res.StatusCode))
			retry = res.StatusCode == 429 || res.StatusCode >= 500
		} else {
			s.set("Webhook network error")
		}
		if !retry {
			return
		}
		timer := time.NewTimer(time.Duration(i+1) * time.Second)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}
