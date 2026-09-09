package videoclip

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	watchdog    = 10 * time.Second // reconnect when no frame arrives for this long
	liveLatency = 2 * time.Second  // the live stream trails the wall clock by about this
	maxBackoff  = 30 * time.Second
	frameGap    = 40 // ms between frames at 25 fps; joins segments after a reconnect
)

// Buffer keeps the last Keep of the live stream in memory so a clip can
// include the seconds before a call on a tariff without cloud recording.
// Frames only, no decoding, nothing on disk: ~3 MiB for 20 s of 1080p.
type Buffer struct {
	Src    *Source      // camera and stream URL resolution (shared with the archive)
	Client *http.Client // no Timeout: the body is read for as long as the streamer allows
	Keep   time.Duration

	mu     sync.Mutex
	frames []frame // ordered by arrival; frames[0] is a keyframe
	fresh  bool    // next frame opens a new connection: stitch its timestamps
	offset uint32  // added to incoming pts/dts so segments join without a jump
	last   uint32  // dts of the last stored frame
	status string
}

// Run streams until ctx is done, reconnecting with backoff.
func (b *Buffer) Run(ctx context.Context) {
	delay := time.Second
	for ctx.Err() == nil {
		got, err := b.stream(ctx)
		if ctx.Err() != nil {
			return
		}
		b.setStatus(err.Error())
		if got {
			delay = time.Second
		}
		if !wait(ctx, delay) {
			return
		}
		if delay < maxBackoff {
			delay *= 2
		}
	}
}

// stream holds one live connection; got reports whether any frame arrived.
// Errors never contain the signed streamer URL.
func (b *Buffer) stream(ctx context.Context) (got bool, err error) {
	camera, err := b.Src.cameraID(ctx)
	if err != nil {
		return false, err
	}
	// ponytail: the light stream (960×528, ~0.45 Mbit/s) runs around the clock;
	// LightStream=0 is 1080p at ~1.4 Mbit/s, three times the traffic.
	streamURL, err := b.Src.URL(camera, url.Values{"LightStream": {"1"}, "Format": {"H264"}})
	if err != nil {
		return false, errors.New("cannot get live URL")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dog := time.AfterFunc(watchdog, cancel) // also bounds connect and headers: the client has no timeout
	defer dog.Stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return false, errors.New("cannot create live request")
	}
	res, err := b.Client.Do(req)
	if err != nil {
		return false, errors.New("live request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("live HTTP %d", res.StatusCode)
	}
	b.mu.Lock()
	b.fresh, b.status = true, "connected"
	b.mu.Unlock()
	err = parseFLV(res.Body, func(f frame) error {
		dog.Reset(watchdog)
		got = true
		b.push(f)
		return nil
	})
	if err == nil {
		err = errors.New("live stream ended")
	}
	return got, err
}

// push appends a frame and drops everything before the last keyframe that
// still covers the Keep window.
func (b *Buffer) push(f frame) {
	f.idr = f.key()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fresh {
		b.fresh = false
		if len(b.frames) > 0 {
			// ponytail: the gap of a reconnect is collapsed, not shown as a pause.
			b.offset = b.last + frameGap - f.dts
		}
	}
	f.pts += b.offset
	f.dts += b.offset
	b.last = f.dts
	b.frames = append(b.frames, f)
	cut := f.at.Add(-b.Keep)
	k := -1
	for i, g := range b.frames {
		if !g.at.Before(cut) {
			break
		}
		if g.idr {
			k = i
		}
	}
	if k > 0 {
		b.frames = append(b.frames[:0], b.frames[k:]...)
	}
}

// Clip waits for the window to pass and returns an MP4 of the buffered
// frames in [start, start+d), starting from the last keyframe before start.
// A partial window (reconnect inside it) still yields a shorter clip.
func (b *Buffer) Clip(ctx context.Context, start time.Time, d time.Duration) ([]byte, error) {
	end := start.Add(d)
	if !wait(ctx, time.Until(end.Add(liveLatency))) {
		return nil, ctx.Err()
	}
	b.mu.Lock()
	k := -1
	for i, f := range b.frames {
		if f.at.After(start) {
			break
		}
		if f.idr {
			k = i
		}
	}
	if k < 0 {
		for i, f := range b.frames {
			if f.idr && !f.at.Before(start) {
				k = i
				break
			}
		}
	}
	var frames []frame
	if k >= 0 {
		for _, f := range b.frames[k:] {
			if !f.at.Before(end) {
				break
			}
			frames = append(frames, f)
		}
	}
	status := b.status
	b.mu.Unlock()
	if len(frames) == 0 {
		return nil, fmt.Errorf("live buffer empty (%s)", status)
	}
	return mux(frames)
}

func (b *Buffer) setStatus(s string) {
	b.mu.Lock()
	b.status = s
	b.mu.Unlock()
}

func wait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
