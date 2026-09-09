package sipclient

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/google/uuid"
	"github.com/moleus/domru/pkg/callcontrol"
	"github.com/moleus/domru/pkg/domru"
	"github.com/stretchr/testify/require"
)

type rig struct {
	t      *testing.T
	s      *Client
	peer   *net.UDPConn
	addr   *net.UDPAddr
	events chan callcontrol.Event
	parser *sip.Parser
}

func newRig(t *testing.T, ring time.Duration) *rig {
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	local, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	addr := local.LocalAddr().(*net.UDPAddr)
	local.Close()
	c, err := New(Config{IP: "127.0.0.1", Port: addr.Port, Installation: uuid.NewString(), RingTime: ring})
	require.NoError(t, err)
	events := make(chan callcontrol.Event, 10)
	c.Ring = func(e callcontrol.Event) { events <- e }
	c.Fetch = func(context.Context) (domru.SIPCredentials, string, error) {
		return domru.SIPCredentials{Login: "test", Password: "secret", Realm: peer.LocalAddr().String()}, "door", nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	r := &rig{t, c, peer, addr, events, sip.NewParser()}
	t.Cleanup(func() {
		cancel()
		peer.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("SIP shutdown stuck")
		}
	})
	req := r.readRequest(sip.REGISTER)
	res := sip.NewResponseFromRequest(req, 401, "Unauthorized", nil)
	res.AppendHeader(sip.NewHeader("WWW-Authenticate", `Digest realm="test", nonce="nonce", algorithm=MD5, qop="auth"`))
	r.send(res.String())
	req = r.readRequest(sip.REGISTER)
	require.NotNil(t, req.GetHeader("Authorization"))
	require.Contains(t, req.GetHeader("Authorization").Value(), "response=")
	res = sip.NewResponseFromRequest(req, 200, "OK", nil)
	res.AppendHeader(sip.NewHeader("Expires", "60"))
	r.send(res.String())
	require.Eventually(t, func() bool { return c.Status().State == "ready" }, time.Second, 10*time.Millisecond)
	return r
}
func (r *rig) send(text string) {
	_, err := r.peer.WriteToUDP([]byte(text), r.addr)
	require.NoError(r.t, err)
}
func (r *rig) read() sip.Message {
	require.NoError(r.t, r.peer.SetReadDeadline(time.Now().Add(3*time.Second)))
	buf := make([]byte, 65535)
	n, _, err := r.peer.ReadFromUDP(buf)
	require.NoError(r.t, err)
	r.t.Logf("recv: %s", strings.SplitN(string(buf[:n]), "\r\n", 2)[0])
	m, err := sip.ParseMessage(buf[:n])
	require.NoError(r.t, err)
	return m
}
func (r *rig) readRequest(method sip.RequestMethod) *sip.Request {
	for {
		m := r.read()
		if req, ok := m.(*sip.Request); ok && req.Method == method {
			return req
		}
	}
}
func (r *rig) response(code sip.StatusCode) *sip.Response {
	for {
		m := r.read()
		if res, ok := m.(*sip.Response); ok && res.StatusCode == code {
			return res
		}
	}
}
func (r *rig) invite(body string) *sip.Request {
	req := sip.NewRequest(sip.INVITE, sip.Uri{Scheme: "sip", User: "test", Host: "127.0.0.1", Port: r.addr.Port})
	req.AppendHeader(sip.NewHeader("Via", fmt.Sprintf("SIP/2.0/UDP %s;branch=z9hG4bK%s;rport", r.peer.LocalAddr(), uuid.NewString())))
	req.AppendHeader(sip.NewHeader("From", "<sip:panel@example.test>;tag=panel"))
	req.AppendHeader(sip.NewHeader("To", "<sip:test@example.test>"))
	req.AppendHeader(sip.NewHeader("Call-ID", uuid.NewString()))
	req.AppendHeader(sip.NewHeader("CSeq", "1 INVITE"))
	req.AppendHeader(sip.NewHeader("Contact", fmt.Sprintf("<sip:panel@%s>", r.peer.LocalAddr())))
	if body != "" {
		req.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
	}
	req.SetBody([]byte(body))
	r.send(req.String())
	return req
}
func (r *rig) follow(inv *sip.Request, res *sip.Response, method sip.RequestMethod, seq int) {
	req := sip.NewRequest(method, inv.Recipient)
	via := sip.HeaderClone(inv.Via())
	if method != sip.CANCEL {
		via = sip.NewHeader("Via", fmt.Sprintf("SIP/2.0/UDP %s;branch=z9hG4bK%s", r.peer.LocalAddr(), uuid.NewString()))
	}
	req.AppendHeader(via)
	req.AppendHeader(sip.HeaderClone(inv.From()))
	if res == nil {
		req.AppendHeader(sip.HeaderClone(inv.To()))
	} else {
		req.AppendHeader(sip.HeaderClone(res.To()))
	}
	req.AppendHeader(sip.HeaderClone(inv.CallID()))
	req.AppendHeader(sip.NewHeader("CSeq", fmt.Sprintf("%d %s", seq, method)))
	req.SetBody(nil)
	r.send(req.String())
}
func TestInviteRetransmitAndCancel(t *testing.T) {
	r := newRig(t, 500*time.Millisecond)
	inv := r.invite("")
	ring := r.response(180)
	e := <-r.events
	r.send(inv.String())
	r.response(180)
	require.Empty(t, r.events)
	r.follow(inv, nil, sip.CANCEL, 1)
	r.response(487)
	require.Eventually(t, func() bool { return r.s.Current() == "" }, time.Second, 10*time.Millisecond)
	require.Equal(t, "canceled", r.s.get(e.ID).state)
	require.ErrorIs(t, r.s.Reject(e.ID), ErrGone)
	// A CANCEL must stop the delayed 486, including after its original deadline.
	time.Sleep(550 * time.Millisecond)
	require.Equal(t, "canceled", r.s.Status().Calls[0].State)
	require.NotEmpty(t, ring.To().Value())
}
func TestRingTimeoutRejectsBranch(t *testing.T) {
	r := newRig(t, 80*time.Millisecond)
	r.invite("")
	r.response(180)
	r.response(486)
	require.Eventually(t, func() bool { return r.s.Current() == "" }, time.Second, 10*time.Millisecond)
}

const offer = "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=panel\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 30000 RTP/AVP 8 0 101\r\na=rtpmap:8 PCMA/8000\r\na=sendrecv\r\n"

func TestAnswerACKBYE(t *testing.T) {
	r := newRig(t, 5*time.Second)
	inv := r.invite(offer)
	r.response(180)
	e := <-r.events
	answer := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		answer <- r.s.Answer(ctx, e.ID)
	}()
	res := r.response(200)
	require.Contains(t, string(res.Body()), "a=recvonly")
	require.Contains(t, string(res.Body()), "PCMA/8000")
	// Missing ACK causes a retransmitted 200 with the same To tag.
	again := r.response(200)
	require.Equal(t, res.To().Value(), again.To().Value())
	r.follow(inv, res, sip.ACK, 1)
	require.NoError(t, <-answer)
	ended := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		ended <- r.s.Bye(ctx, e.ID)
	}()
	bye := r.readRequest(sip.BYE)
	require.Equal(t, inv.CallID().Value(), bye.CallID().Value())
	require.Equal(t, res.To().Params["tag"], bye.From().Params["tag"])
	r.send(sip.NewResponseFromRequest(bye, 200, "OK", nil).String())
	require.NoError(t, <-ended)
	require.Empty(t, r.s.Current())
}
func TestRemoteBYE(t *testing.T) {
	r := newRig(t, 5*time.Second)
	inv := r.invite(offer)
	r.response(180)
	e := <-r.events
	done := make(chan error, 1)
	go func() { done <- r.s.Answer(context.Background(), e.ID) }()
	res := r.response(200)
	r.follow(inv, res, sip.ACK, 1)
	require.NoError(t, <-done)
	r.follow(inv, res, sip.BYE, 2)
	r.response(200)
	require.Eventually(t, func() bool { return r.s.Current() == "" }, time.Second, 10*time.Millisecond)
}
func TestSDPAndIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id")
	first, err := InstallationID(path)
	require.NoError(t, err)
	second, err := InstallationID(path)
	require.NoError(t, err)
	require.Equal(t, first, second)
	for _, bad := range []string{"", strings.ReplaceAll(offer, "RTP/AVP", "RTP/SAVP"), strings.ReplaceAll(offer, "a=sendrecv", "a=recvonly"), offer + "a=ice-ufrag:unsupported\r\n", strings.ReplaceAll(strings.ReplaceAll(offer, "8 0 101", "96"), "8 PCMA/8000", "96 opus/48000/2")} {
		_, _, err := answerSDP([]byte(bad), "127.0.0.1", 22000, 22100)
		require.Error(t, err)
	}
	for _, codec := range []string{"PCMA", "PCMU"} {
		sdp := strings.ReplaceAll(offer, "PCMA", codec)
		body, sink, err := answerSDP([]byte(sdp), "127.0.0.1", 22000, 22100)
		require.NoError(t, err)
		require.Contains(t, string(body), codec)
		conn, err := net.Dial("udp", sink.rtp.LocalAddr().String())
		require.NoError(t, err)
		_, err = conn.Write([]byte{0x80, 8, 0, 1, 0, 0, 0, 0, 0, 0, 0, 1})
		require.NoError(t, err)
		conn.Close()
		sink.Close()
	}
}
