// Package sipclient implements the optional, single-intercom UDP experiment.
package sipclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/google/uuid"
	"github.com/moleus/domru/pkg/callcontrol"
	"github.com/moleus/domru/pkg/domru"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
)

var ErrGone = errors.New("call no longer available")

type Config struct {
	IP                string
	Port              int
	Installation      string
	RTPFirst, RTPLast int
	RingTime          time.Duration
}
type Status struct {
	State string       `json:"state"`
	Error string       `json:"error,omitempty"`
	Calls []CallStatus `json:"calls"`
}
type CallStatus struct {
	ID      string    `json:"id"`
	State   string    `json:"state"`
	Started time.Time `json:"started"`
}
type call struct {
	mu             sync.Mutex
	id, key, state string
	started        time.Time
	req            *sip.Request
	tx             sip.ServerTransaction
	response       *sip.Response
	canceled       atomic.Bool
	cancelCh       chan struct{}
	ack            chan struct{}
	end            chan struct{}
	timer          *time.Timer
	media          *mediaSink
}

type Client struct {
	cfg                           Config
	Fetch                         func(context.Context) (domru.SIPCredentials, string, error)
	Ring                          func(callcontrol.Event)
	mu                            sync.Mutex
	state, lastError, name, login string
	peers                         map[string]bool
	calls                         map[string]*call
	ua                            *sipgo.UserAgent
	client                        *sipgo.Client
}

func New(cfg Config) (*Client, error) {
	if net.ParseIP(cfg.IP).To4() == nil || net.ParseIP(cfg.IP).IsUnspecified() || cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("SIP requires an explicit local IPv4 address and UDP port")
	}
	if cfg.RingTime == 0 {
		cfg.RingTime = 25 * time.Second
	}
	if cfg.RTPFirst == 0 {
		cfg.RTPFirst = 20000
		cfg.RTPLast = 20100
	}
	if cfg.RTPFirst < 1024 || cfg.RTPFirst%2 != 0 || cfg.RTPLast > 65535 || cfg.RTPLast <= cfg.RTPFirst {
		return nil, fmt.Errorf("invalid RTP port range")
	}
	// sipgo v0.28 uses a package-global wire logger in its transport/parser.
	// Disable it before creating any UA; expose only our credential-free status.
	zlog.Logger = zerolog.Nop()
	return &Client{cfg: cfg, state: "waiting_auth", calls: make(map[string]*call), peers: make(map[string]bool)}, nil
}
func (s *Client) setStatus(state, msg string) {
	s.mu.Lock()
	s.state, s.lastError = state, msg
	s.mu.Unlock()
}
func (s *Client) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Status{State: s.state, Error: s.lastError, Calls: []CallStatus{}}
	for _, c := range s.calls {
		c.mu.Lock()
		out.Calls = append(out.Calls, CallStatus{c.id, c.state, c.started})
		c.mu.Unlock()
	}
	return out
}
func (s *Client) Current() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		c.mu.Lock()
		active := c.state == "ringing" || c.state == "answering" || c.state == "established"
		c.mu.Unlock()
		if active && !c.canceled.Load() {
			return c.id
		}
	}
	return ""
}
func (s *Client) get(id string) *call { s.mu.Lock(); defer s.mu.Unlock(); return s.calls[id] }
func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Client) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := s.serve(ctx); err != nil && ctx.Err() == nil {
			s.setStatus("error", "SIP transport unavailable")
		}
		if !pause(ctx, 5*time.Second) {
			break
		}
	}
	s.setStatus("off", "")
}
func (s *Client) serve(ctx context.Context) error {
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("domru"))
	if err != nil {
		return err
	}
	defer ua.Close()
	cli, err := sipgo.NewClient(ua, sipgo.WithClientHostname(s.cfg.IP), sipgo.WithClientPort(s.cfg.Port), sipgo.WithClientNAT())
	if err != nil {
		return err
	}
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return err
	}
	conn, err := net.ListenPacket("udp4", net.JoinHostPort(s.cfg.IP, strconv.Itoa(s.cfg.Port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	s.mu.Lock()
	s.ua, s.client = ua, cli
	s.mu.Unlock()
	srv.OnInvite(s.invite)
	srv.OnAck(s.ack)
	srv.OnBye(s.remoteBye)
	srv.OnCancel(func(r *sip.Request, t sip.ServerTransaction) {
		_ = t.Respond(sip.NewResponseFromRequest(r, 481, "Call Does Not Exist", nil))
	})
	ok := func(r *sip.Request, t sip.ServerTransaction) {
		if s.allowed(r) {
			_ = t.Respond(sip.NewResponseFromRequest(r, 200, "OK", nil))
		} else {
			_ = t.Respond(sip.NewResponseFromRequest(r, 403, "Forbidden", nil))
		}
	}
	srv.OnOptions(ok)
	srv.OnNotify(ok)
	serving := make(chan error, 1)
	go func() { serving <- ua.TransportLayer().ServeUDP(conn) }()
	// sipgo adds the listener to its connection pool asynchronously. A request
	// sent before that binds a second socket to the same port and fails, which
	// delays the first REGISTER by a full retry interval.
	for i := 0; i < 200; i++ {
		if c, _ := ua.TransportLayer().GetConnection("udp", conn.LocalAddr().String()); c != nil {
			_, _ = c.TryClose()
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	local, cancel := context.WithCancel(ctx)
	defer cancel()
	registered := make(chan struct{})
	go func() { defer close(registered); s.register(local, cli) }()
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case err = <-serving:
	}
	cancel()
	conn.Close()
	<-registered
	s.mu.Lock()
	calls := make([]*call, 0, len(s.calls))
	for _, c := range s.calls {
		calls = append(calls, c)
	}
	s.mu.Unlock()
	for _, c := range calls {
		c.mu.Lock()
		s.finishLocked(c, "ended")
		c.mu.Unlock()
	}
	return err
}
func (s *Client) allowed(r *sip.Request) bool {
	host, _, err := net.SplitHostPort(r.Source())
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peers[host]
}
func (s *Client) register(ctx context.Context, cli *sipgo.Client) {
	for ctx.Err() == nil {
		s.setStatus("waiting_auth", "")
		fetch, cancel := context.WithTimeout(ctx, 15*time.Second)
		cred, name, err := s.Fetch(fetch)
		cancel()
		if err != nil {
			s.setStatus("waiting_auth", "Unable to obtain SIP credentials or validate intercom")
			if !pause(ctx, 30*time.Second) {
				return
			}
			continue
		}
		var target sip.Uri
		realm := cred.Realm
		if !strings.HasPrefix(realm, "sip:") {
			realm = "sip:" + realm
		}
		if sip.ParseUri(realm, &target) != nil || target.Host == "" || target.User != "" || strings.ContainsAny(cred.Login, "\r\n<>@ ") {
			s.setStatus("error", "Invalid SIP credentials format")
			if !pause(ctx, 30*time.Second) {
				return
			}
			continue
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, target.Host)
		if err != nil {
			s.setStatus("error", "Cannot resolve SIP registrar")
			if !pause(ctx, 5*time.Second) {
				return
			}
			continue
		}
		s.mu.Lock()
		s.peers = map[string]bool{}
		for _, ip := range ips {
			s.peers[ip.IP.String()] = true
		}
		s.login, s.name = cred.Login, name
		s.mu.Unlock()
		req := sip.NewRequest(sip.REGISTER, target)
		req.SetTransport("UDP")
		address := target
		address.User = cred.Login
		req.AppendHeader(&sip.FromHeader{Address: address, Params: sip.HeaderParams{"tag": uuid.NewString()}})
		req.AppendHeader(&sip.ToHeader{Address: address, Params: sip.NewParams()})
		expires := 60
		for ctx.Err() == nil {
			s.setStatus("registering", "")
			req.RemoveHeader("Via")
			req.RemoveHeader("Authorization")
			req.RemoveHeader("Proxy-Authorization")
			req.RemoveHeader("Contact")
			req.RemoveHeader("Expires")
			req.AppendHeader(sip.NewHeader("Contact", fmt.Sprintf("<sip:%s@%s:%d>;expires=%d;+sip.instance=\"<urn:uuid:%s>\"", cred.Login, s.cfg.IP, s.cfg.Port, expires, s.cfg.Installation)))
			req.AppendHeader(sip.NewHeader("Expires", strconv.Itoa(expires)))
			op, cancel := context.WithTimeout(ctx, 10*time.Second)
			res, err := cli.Do(op, req)
			if err == nil && res != nil && (res.StatusCode == 401 || res.StatusCode == 407) {
				var tx sip.ClientTransaction
				tx, err = cli.DoDigestAuth(op, req, res, sipgo.DigestAuth{Username: cred.Login, Password: cred.Password})
				if err == nil {
					res, err = waitResponse(op, tx)
					tx.Terminate()
				}
			}
			cancel()
			if err != nil || res == nil {
				s.setStatus("error", "SIP registration timed out or network unavailable")
				break
			}
			if res.StatusCode == 423 {
				if h := res.GetHeader("Min-Expires"); h != nil {
					n, e := strconv.Atoi(h.Value())
					if e == nil && n > 0 && n <= 86400 {
						expires = n
						continue
					}
				}
			}
			if res.StatusCode != 200 {
				s.setStatus("error", fmt.Sprintf("SIP registration returned %d", res.StatusCode))
				break
			}
			ttl := expires
			if h := res.GetHeader("Expires"); h != nil {
				if n, e := strconv.Atoi(h.Value()); e == nil {
					ttl = n
				}
			}
			if h := res.Contact(); h != nil {
				if value, ok := h.Params.Get("expires"); ok {
					if n, e := strconv.Atoi(value); e == nil {
						ttl = n
					}
				}
			}
			if ttl <= 0 {
				s.setStatus("error", "SIP registration not granted")
				break
			}
			s.setStatus("ready", "")
			if !pause(ctx, time.Duration(ttl)*time.Second*4/5) {
				return
			}
		}
		if !pause(ctx, 5*time.Second) {
			return
		}
	}
}
func waitResponse(ctx context.Context, tx sip.ClientTransaction) (*sip.Response, error) {
	for {
		select {
		case r := <-tx.Responses():
			if r != nil && !r.IsProvisional() {
				return r, nil
			}
		case <-tx.Done():
			return nil, errors.New("SIP transaction ended")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func requestKey(r *sip.Request) string {
	if r.CallID() == nil || r.From() == nil || r.CSeq() == nil {
		return ""
	}
	tag, _ := r.From().Params.Get("tag")
	return r.CallID().Value() + "/" + tag
}
func (s *Client) invite(r *sip.Request, tx sip.ServerTransaction) {
	if !s.allowed(r) {
		_ = tx.Respond(sip.NewResponseFromRequest(r, 403, "Forbidden", nil))
		return
	}
	if r.To() == nil || r.Contact() == nil || requestKey(r) == "" {
		_ = tx.Respond(sip.NewResponseFromRequest(r, 400, "Bad Request", nil))
		return
	}
	if tag, _ := r.To().Params.Get("tag"); tag != "" {
		_ = tx.Respond(sip.NewResponseFromRequest(r, 488, "Not Acceptable Here", nil))
		return
	}
	s.mu.Lock()
	for _, old := range s.calls {
		old.mu.Lock()
		if old.key == requestKey(r) {
			res := old.response
			old.mu.Unlock()
			s.mu.Unlock()
			if res != nil {
				copy := sip.NewResponseFromRequest(r, res.StatusCode, res.Reason, res.Body())
				copy.To().Params = res.To().Params.Clone()
				if h := res.Contact(); h != nil {
					copy.AppendHeader(sip.HeaderClone(h))
				}
				if h := res.GetHeader("Content-Type"); h != nil {
					copy.AppendHeader(sip.HeaderClone(h))
				}
				_ = tx.Respond(copy)
			}
			return
		}
		active := old.state == "ringing" || old.state == "answering" || old.state == "established"
		old.mu.Unlock()
		if active {
			s.mu.Unlock()
			_ = tx.Respond(sip.NewResponseFromRequest(r, 486, "Busy Here", nil))
			return
		}
	}
	c := &call{id: uuid.NewString(), key: requestKey(r), state: "ringing", started: time.Now(), req: r, tx: tx, ack: make(chan struct{}), cancelCh: make(chan struct{}, 1), end: make(chan struct{})}
	r.To().Params.Add("tag", uuid.NewString())
	s.calls[c.id] = c
	name, login := s.name, s.login
	s.mu.Unlock()
	if st, ok := tx.(*sip.ServerTx); ok {
		st.OnCancel(func(_ *sip.Request) {
			c.canceled.Store(true)
			select {
			case c.cancelCh <- struct{}{}:
			default:
			}
		})
	}
	c.mu.Lock()
	c.response = sip.NewResponseFromRequest(r, 180, "Ringing", nil)
	c.response.AppendHeader(sip.NewHeader("Contact", fmt.Sprintf("<sip:%s@%s:%d>", login, s.cfg.IP, s.cfg.Port)))
	err := tx.Respond(c.response)
	if err != nil {
		s.finishLocked(c, "error")
	} else {
		c.timer = time.AfterFunc(s.cfg.RingTime, func() { _ = s.Reject(c.id) })
	}
	c.mu.Unlock()
	if err == nil && s.Ring != nil {
		s.Ring(callcontrol.Event{ID: c.id, Time: c.started, Name: name})
	}
	// Keep tombstones long enough to deduplicate retransmissions, but bound memory.
	go func() { <-c.end; time.Sleep(2 * time.Minute); s.mu.Lock(); delete(s.calls, c.id); s.mu.Unlock() }()
	// sipgo terminates the server transaction as soon as this handler returns,
	// so it must block until the call is over (each request runs in its own goroutine).
	for {
		select {
		case <-c.cancelCh:
			c.mu.Lock()
			s.finishLocked(c, "canceled")
			c.mu.Unlock()
			s.drain(c, tx)
			return
		case ack := <-tx.Acks():
			if ack != nil {
				s.ack(ack, nil)
			}
		case <-tx.Done():
			c.mu.Lock()
			if c.state == "ringing" || c.state == "answering" {
				s.finishLocked(c, "ended")
			}
			c.mu.Unlock()
			<-c.end
			return
		case <-c.end:
			s.drain(c, tx)
			return
		}
	}
}

// drain lets sipgo finish final-response retransmissions (timers G/H/L) before
// the handler returns and the transaction is terminated. ACKs are still
// consumed so the 2xx retransmission in Answer stops.
func (s *Client) drain(c *call, tx sip.ServerTransaction) {
	limit := time.NewTimer(35 * time.Second)
	defer limit.Stop()
	for {
		select {
		case ack := <-tx.Acks():
			if ack != nil {
				s.ack(ack, nil)
			}
		case <-tx.Done():
			return
		case <-limit.C:
			return
		}
	}
}
func (s *Client) finishLocked(c *call, state string) {
	select {
	case <-c.end:
		return
	default:
	}
	c.state = state
	if c.timer != nil {
		c.timer.Stop()
	}
	if c.media != nil {
		c.media.Close()
	}
	close(c.end)
}
func (s *Client) Reject(id string) error {
	c := s.get(id)
	if c == nil {
		return ErrGone
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != "ringing" || c.canceled.Load() {
		return ErrGone
	}
	c.response = sip.NewResponseFromRequest(c.req, 486, "Busy Here", nil)
	err := c.tx.Respond(c.response)
	s.finishLocked(c, "rejected")
	return err
}
func (s *Client) Answer(ctx context.Context, id string) error {
	c := s.get(id)
	if c == nil {
		return ErrGone
	}
	c.mu.Lock()
	if c.state != "ringing" || c.canceled.Load() {
		c.mu.Unlock()
		return ErrGone
	}
	ct := c.req.GetHeader("Content-Type")
	if ct == nil || !strings.EqualFold(strings.TrimSpace(strings.Split(ct.Value(), ";")[0]), "application/sdp") {
		c.mu.Unlock()
		return fmt.Errorf("SDP offer required")
	}
	body, media, err := answerSDP(c.req.Body(), s.cfg.IP, s.cfg.RTPFirst, s.cfg.RTPLast)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.media = media
	c.state = "answering"
	c.timer.Stop()
	res := sip.NewResponseFromRequest(c.req, 200, "OK", body)
	res.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
	res.AppendHeader(sip.HeaderClone(c.response.Contact()))
	c.response = res
	err = c.tx.Respond(res)
	if err != nil {
		s.finishLocked(c, "error")
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()
	// UAS retransmits the 2xx until an ACK, independently of INVITE retransmits.
	deadline := time.NewTimer(32 * time.Second)
	defer deadline.Stop()
	interval := 500 * time.Millisecond
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-c.ack:
			return nil
		case <-c.end:
			return ErrGone
		case <-ctx.Done():
			s.abortAnswer(c)
			return ctx.Err()
		case <-deadline.C:
			s.abortAnswer(c)
			return fmt.Errorf("ACK not received")
		case <-timer.C:
			c.mu.Lock()
			if c.state == "answering" {
				_ = c.tx.Respond(c.response)
			}
			c.mu.Unlock()
			if interval < 4*time.Second {
				interval *= 2
			}
			timer.Reset(interval)
		}
	}
}
func (s *Client) abortAnswer(c *call) {
	// Keep retransmitting in the background through the SIP ACK timeout even
	// when the HTTP caller disconnects, then release the dialog by BYE.
	go func() {
		timeout := time.NewTimer(32 * time.Second)
		defer timeout.Stop()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-c.ack:
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				_ = s.Bye(ctx, c.id)
				cancel()
				return
			case <-c.end:
				return
			case <-timeout.C:
				c.mu.Lock()
				s.finishLocked(c, "ack_timeout")
				c.mu.Unlock()
				return
			case <-tick.C:
				c.mu.Lock()
				if c.state == "answering" {
					_ = c.tx.Respond(c.response)
				}
				c.mu.Unlock()
			}
		}
	}()
}
func matches(c *call, r *sip.Request) bool {
	if requestKey(r) != c.key || r.To() == nil {
		return false
	}
	a, _ := r.To().Params.Get("tag")
	b, _ := c.req.To().Params.Get("tag")
	return a == b
}
func (s *Client) ack(r *sip.Request, _ sip.ServerTransaction) {
	if !s.allowed(r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		c.mu.Lock()
		if matches(c, r) && c.state == "answering" && r.CSeq().SeqNo == c.req.CSeq().SeqNo {
			c.state = "established"
			close(c.ack)
		}
		c.mu.Unlock()
	}
}
func (s *Client) remoteBye(r *sip.Request, tx sip.ServerTransaction) {
	if !s.allowed(r) {
		_ = tx.Respond(sip.NewResponseFromRequest(r, 403, "Forbidden", nil))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		c.mu.Lock()
		if matches(c, r) && (c.state == "established" || c.state == "answering") && r.CSeq().SeqNo > c.req.CSeq().SeqNo {
			_ = tx.Respond(sip.NewResponseFromRequest(r, 200, "OK", nil))
			s.finishLocked(c, "ended")
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}
	_ = tx.Respond(sip.NewResponseFromRequest(r, 481, "Call Does Not Exist", nil))
}
func (s *Client) Bye(ctx context.Context, id string) error {
	c := s.get(id)
	if c == nil {
		return ErrGone
	}
	c.mu.Lock()
	if c.state != "established" {
		c.mu.Unlock()
		return ErrGone
	}
	req := sip.NewRequest(sip.BYE, c.req.Contact().Address)
	from := c.req.To().AsFrom()
	to := c.req.From().AsTo()
	req.AppendHeader(&from)
	req.AppendHeader(&to)
	req.AppendHeader(sip.HeaderClone(c.req.CallID()))
	req.AppendHeader(&sip.CSeqHeader{SeqNo: c.req.CSeq().SeqNo, MethodName: sip.BYE})
	// The UAS route set keeps the order from the INVITE (RFC 3261 12.1.1).
	for _, h := range c.req.GetHeaders("Record-Route") {
		req.AppendHeader(sip.NewHeader("Route", h.Value()))
	}
	if route := req.Route(); route != nil {
		if !route.Address.UriParams.Has("lr") {
			c.mu.Unlock()
			return fmt.Errorf("strict SIP routing unsupported")
		}
		req.SetDestination(route.Address.HostPort())
	} else {
		req.SetDestination(c.req.Source())
	}
	req.SetTransport("UDP")
	c.state = "ending"
	c.mu.Unlock()
	s.mu.Lock()
	cli := s.client
	s.mu.Unlock()
	res, err := cli.Do(ctx, req)
	c.mu.Lock()
	defer c.mu.Unlock()
	s.finishLocked(c, "ended")
	if err != nil || res == nil || res.StatusCode != 200 {
		return fmt.Errorf("BYE not acknowledged")
	}
	return nil
}
