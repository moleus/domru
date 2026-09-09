package videoclip

import (
	"context"
	"sync"
	"time"
)

// Auto serves clips from the archive and switches to the live Buffer for
// good once the streamer answers a TS request with live video, i.e. the
// tariff has no recording. The switch happens inside the call that
// detected it, so that call still gets the seconds after the ring.
type Auto struct {
	Archive *Source
	Buffer  *Buffer
	Ctx     context.Context // lifetime of the buffer once started

	once sync.Once
	mu   sync.Mutex
	live bool
}

// Probe checks the archive once at startup so the buffer is already warm
// for the first call on a tariff without recording.
func (a *Auto) Probe(ctx context.Context) {
	if _, err := a.Archive.Clip(ctx, time.Now().Add(-30*time.Second), time.Second); err == errLive {
		a.useBuffer()
	}
}

// Clip implements the telegram.Bot Video hook.
func (a *Auto) Clip(ctx context.Context, start time.Time, d time.Duration) ([]byte, error) {
	a.mu.Lock()
	live := a.live
	a.mu.Unlock()
	if !live {
		clip, err := a.Archive.Clip(ctx, start, d)
		if err != errLive {
			return clip, err
		}
		a.useBuffer()
	}
	return a.Buffer.Clip(ctx, start, d)
}

// ponytail: the switch lasts until restart; a subscription bought later needs one.
func (a *Auto) useBuffer() {
	a.mu.Lock()
	a.live = true
	a.mu.Unlock()
	a.once.Do(func() {
		a.Buffer.setStatus("archive unavailable, using live buffer")
		ctx := a.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		go a.Buffer.Run(ctx)
	})
}
