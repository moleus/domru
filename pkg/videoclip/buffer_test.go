package videoclip

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	codec "github.com/yapingcat/gomedia/go-codec"
	mp4 "github.com/yapingcat/gomedia/go-mp4"
)

// feed pushes the frames of fakeFLV into b as if they arrived one every 40 ms
// starting at t0; it returns the arrival time of the last frame.
func feed(t *testing.T, b *Buffer, frames int, base uint32, t0 time.Time) time.Time {
	var at time.Time
	i := 0
	err := parseFLV(bytes.NewReader(fakeFLV(t, frames, base)), func(f frame) error {
		at = t0.Add(time.Duration(i) * 40 * time.Millisecond)
		f.at = at
		i++
		b.push(f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func demux(t *testing.T, clip []byte) (frames int, firstDts, lastDts uint64) {
	d := mp4.CreateMp4Demuxer(bytes.NewReader(clip))
	if _, err := d.ReadHead(); err != nil {
		t.Fatalf("mp4 head: %v", err)
	}
	firstDts = ^uint64(0)
	for {
		pkg, err := d.ReadPacket()
		if err != nil {
			break
		}
		if pkg.Cid != mp4.MP4_CODEC_H264 {
			continue
		}
		frames++
		if pkg.Dts < firstDts {
			firstDts = pkg.Dts
		}
		lastDts = pkg.Dts
	}
	return
}

func TestBufferTrimsToKeyframe(t *testing.T) {
	b := &Buffer{Keep: 20 * time.Second}
	b.fresh = true
	t0 := time.Now().Add(-time.Minute)
	last := feed(t, b, 60*25, 0, t0)
	if !b.frames[0].idr {
		t.Fatal("buffer does not start with a keyframe")
	}
	cut := last.Add(-b.Keep)
	if b.frames[0].at.After(cut) {
		t.Fatalf("window not covered: first frame %v after cut %v", b.frames[0].at, cut)
	}
	// keyframes come every second, so at most one second of extra history
	if cut.Sub(b.frames[0].at) > time.Second {
		t.Fatalf("too much history kept: %v", cut.Sub(b.frames[0].at))
	}
}

func TestBufferClipCutsWindow(t *testing.T) {
	b := &Buffer{Keep: 20 * time.Second}
	b.fresh = true
	t0 := time.Now().Add(-time.Minute)
	feed(t, b, 30*25, 500, t0)
	start := t0.Add(10*time.Second + 500*time.Millisecond) // inside a GOP: clip starts at the keyframe before
	clip, err := b.Clip(context.Background(), start, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	frames, first, last := demux(t, clip)
	if first != 0 {
		t.Fatalf("clip should start at dts 0, got %d", first)
	}
	// keyframe at 10.0 s through 15.5 s exclusive → 5.5 s of video
	if frames < 135 || frames > 140 || last < 5300 || last > 5500 {
		t.Fatalf("unexpected clip: %d frames, last dts %d", frames, last)
	}
}

func TestBufferReconnectStitchesDTS(t *testing.T) {
	b := &Buffer{Keep: time.Minute}
	b.fresh = true
	t0 := time.Now().Add(-time.Minute)
	last := feed(t, b, 5*25, 100_000_000, t0)
	b.fresh = true // new connection with a different timestamp base and a 3 s gap
	feed(t, b, 5*25, 700, last.Add(3*time.Second))
	prev := b.frames[0].dts
	for _, f := range b.frames[1:] {
		if f.dts != prev+frameGap {
			t.Fatalf("dts jump %d → %d", prev, f.dts)
		}
		prev = f.dts
	}
}

func TestBufferEmpty(t *testing.T) {
	b := &Buffer{Keep: 20 * time.Second}
	b.setStatus("live request failed")
	_, err := b.Clip(context.Background(), time.Now().Add(-time.Minute), time.Second)
	if err == nil || !strings.Contains(err.Error(), "live request failed") {
		t.Fatalf("want empty-buffer error with status, got %v", err)
	}
	if strings.Contains(err.Error(), "http") {
		t.Fatalf("error leaks URL: %v", err)
	}
}

func TestBufferStreamFillsFrames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("TS") != "" {
			t.Error("live buffer must not request the archive")
		}
		_, _ = w.Write(fakeFLV(t, 3*25, 433696720))
	}))
	defer srv.Close()
	src := &Source{
		Camera: func(context.Context) (string, error) { return "cam", nil },
		URL:    func(camera string, q url.Values) (string, error) { return srv.URL + "/live?" + q.Encode(), nil },
	}
	b := &Buffer{Src: src, Client: srv.Client(), Keep: 20 * time.Second}
	got, err := b.stream(context.Background())
	if !got || err == nil || err.Error() != "live stream ended" {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if len(b.frames) != 75 || !b.frames[0].idr || b.frames[0].cid != codec.CODECID_VIDEO_H264 {
		t.Fatalf("frames=%d", len(b.frames))
	}
}

func TestAutoFallsBackToBufferOnLive(t *testing.T) {
	var archiveHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("TS") != "" {
			atomic.AddInt32(&archiveHits, 1)
		}
		_, _ = w.Write(fakeFLV(t, 3*25, 433696720)) // live timestamps either way
	}))
	defer srv.Close()
	src := &Source{
		Camera: func(context.Context) (string, error) { return "cam", nil },
		URL:    func(camera string, q url.Values) (string, error) { return srv.URL + "/v?" + q.Encode(), nil },
		Client: srv.Client(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &Auto{Archive: src, Buffer: &Buffer{Src: src, Client: srv.Client(), Keep: 20 * time.Second}, Ctx: ctx}
	// Like a real call: the window still runs when the archive turns out to be
	// live, the buffer started inside Clip collects the tail of the window.
	clip, err := a.Clip(ctx, time.Now().Add(-30*time.Second), 31*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if frames, _, _ := demux(t, clip); frames == 0 {
		t.Fatal("empty clip from buffer")
	}
	if !a.live || atomic.LoadInt32(&archiveHits) != 1 {
		t.Fatalf("live=%v archive hits=%d", a.live, archiveHits)
	}
	if _, err := a.Clip(ctx, time.Now().Add(-30*time.Second), 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&archiveHits) != 1 {
		t.Fatalf("second call went to the archive again: %d hits", archiveHits)
	}
}

func TestAutoKeepsArchiveOnNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	src := &Source{
		Camera: func(context.Context) (string, error) { return "cam", nil },
		URL:    func(camera string, q url.Values) (string, error) { return srv.URL + "/v?" + q.Encode(), nil },
		Client: srv.Client(),
	}
	a := &Auto{Archive: src, Buffer: &Buffer{Src: src, Client: srv.Client(), Keep: 20 * time.Second}, Ctx: context.Background()}
	_, err := a.Clip(context.Background(), time.Now().Add(-30*time.Second), 30*time.Second)
	if err == nil || errors.Is(err, errLive) || a.live {
		t.Fatalf("archive must stay: err=%v live=%v", err, a.live)
	}
}
