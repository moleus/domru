package authorizedhttp

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type tokens struct {
	mu        sync.Mutex
	token     string
	refreshes int
	closed    *bool
}

func (t *tokens) GetToken() (string, error)   { t.mu.Lock(); defer t.mu.Unlock(); return t.token, nil }
func (t *tokens) GetOperatorID() (int, error) { return 7, nil }
func (t *tokens) RefreshToken() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed != nil && !*t.closed {
		return errors.New("previous body not closed")
	}
	t.refreshes++
	t.token = "new"
	return nil
}

type doFunc func(*http.Request) (*http.Response, error)

func (f doFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

type trackedBody struct {
	io.Reader
	closed *bool
}

func (b trackedBody) Close() error { *b.closed = true; return nil }
func TestPOSTReplayClosesResponse(t *testing.T) {
	closed := false
	p := &tokens{token: "old", closed: &closed}
	c := NewClient(p, p, p)
	var bodies []string
	c.DefaultClient = doFunc(func(r *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(r.Body)
		r.Body.Close()
		bodies = append(bodies, string(data))
		status := 200
		if len(bodies) == 1 {
			status = 401
		}
		return &http.Response{StatusCode: status, Body: trackedBody{strings.NewReader(""), &closed}}, nil
	})
	req := httptest.NewRequest("POST", "http://upstream/actions", strings.NewReader(`{"name":"accessControlOpen"}`))
	res, err := c.Do(req)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, []string{`{"name":"accessControlOpen"}`, `{"name":"accessControlOpen"}`}, bodies)
	require.Empty(t, req.Header.Get("Authorization"))
	require.Equal(t, 1, p.refreshes)
}
func TestConcurrent401RefreshOnce(t *testing.T) {
	p := &tokens{token: "old"}
	c := NewClient(p, p, p)
	const n = 16
	var reached sync.WaitGroup
	reached.Add(n)
	c.DefaultClient = doFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") == "Bearer old" {
			reached.Done()
			reached.Wait()
			return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	var workers sync.WaitGroup
	workers.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer workers.Done()
			req, _ := http.NewRequest("GET", "http://upstream", nil)
			res, err := c.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			res.Body.Close()
		}()
	}
	workers.Wait()
	require.Equal(t, 1, p.refreshes)
}
func TestNetworkErrorDoesNotRetry(t *testing.T) {
	p := &tokens{token: "old"}
	c := NewClient(p, p, p)
	count := 0
	c.DefaultClient = doFunc(func(*http.Request) (*http.Response, error) { count++; return nil, errors.New("lost response") })
	_, err := c.Do(httptest.NewRequest("POST", "http://upstream", strings.NewReader("open")))
	require.Error(t, err)
	require.Equal(t, 1, count)
	require.Zero(t, p.refreshes)
}
