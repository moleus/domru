// Package videoclip cuts a short MP4 out of the Дом.ру cloud archive.
//
// The operator keeps a continuous recording per camera; a request with
// TS=<unix seconds> plays that recording from the given moment as HTTP-FLV in
// real time. One request per call therefore replaces any in-memory ring
// buffer of the live stream. The FLV (H.264 + MP3) is remuxed into MP4 in
// memory so Telegram plays it inline.
package videoclip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	codec "github.com/yapingcat/gomedia/go-codec"
	flv "github.com/yapingcat/gomedia/go-flv"
	mp4 "github.com/yapingcat/gomedia/go-mp4"
)

// maxBody bounds one clip download; 30 s of the 1080p stream is ~5 MiB.
const maxBody = 32 << 20

// liveThreshold separates archive playback (timestamps start near zero) from
// the live stream the streamer silently falls back to when TS is not covered
// by the recording (timestamps in the hundreds of millions).
const liveThreshold = time.Hour

var (
	errLive  = errors.New("archive unavailable, streamer returned live video")
	errShort = errors.New("archive stream ended early")
	errDone  = errors.New("done")
)

// Source resolves the camera of the configured intercom once and fetches
// archive clips through the operator API.
type Source struct {
	Camera func(ctx context.Context) (string, error)
	URL    func(camera string, query url.Values) (string, error)
	Client *http.Client

	mu     sync.Mutex
	camera string
}

// Clip returns an MP4 covering [start, start+d) or an error without the
// signed streamer URL in it.
func (s *Source) Clip(ctx context.Context, start time.Time, d time.Duration) ([]byte, error) {
	camera, err := s.cameraID(ctx)
	if err != nil {
		return nil, err
	}
	query := url.Values{"TS": {strconv.FormatInt(start.Unix(), 10)}, "LightStream": {"0"}, "Format": {"H264"}}
	streamURL, err := s.URL(camera, query)
	if err != nil {
		return nil, errors.New("cannot get archive URL")
	}
	// The archive plays in real time, so the download takes about d itself.
	ctx, cancel := context.WithTimeout(ctx, d+40*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return nil, errors.New("cannot create archive request")
	}
	res, err := s.Client.Do(req)
	if err != nil {
		return nil, errors.New("archive request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archive HTTP %d", res.StatusCode)
	}
	return Remux(io.LimitReader(res.Body, maxBody), d)
}

func (s *Source) cameraID(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.camera == "" {
		camera, err := s.Camera(ctx)
		if err != nil {
			return "", err
		}
		s.camera = camera
	}
	return s.camera, nil
}

// Remux reads an FLV stream and returns an MP4 with the first d of video
// (plus the MP3 audio in that window). Fewer than d of video is an error:
// the caller retries once the archive has caught up.
func Remux(r io.Reader, d time.Duration) ([]byte, error) {
	var out memWriter
	muxer, err := mp4.CreateMp4Muxer(&out)
	if err != nil {
		return nil, errors.New("cannot create MP4 muxer")
	}
	reader := flv.CreateFlvReader()
	var video, audio uint32
	var hasVideo, hasAudio bool
	var first uint32
	limit := uint32(d / time.Millisecond)
	var stop error
	reader.OnFrame = func(cid codec.CodecID, frame []byte, pts, dts uint32) {
		if stop != nil {
			return
		}
		switch cid {
		case codec.CODECID_VIDEO_H264:
			if !hasVideo {
				if time.Duration(dts)*time.Millisecond > liveThreshold {
					stop = errLive
					return
				}
				video = muxer.AddVideoTrack(mp4.MP4_CODEC_H264)
				hasVideo, first = true, dts
			}
			if dts-first >= limit {
				stop = errDone
				return
			}
			stop = muxer.Write(video, frame, uint64(pts), uint64(dts))
		case codec.CODECID_AUDIO_MP3:
			if !hasVideo {
				return // align audio with the first video frame
			}
			if !hasAudio {
				audio = muxer.AddAudioTrack(mp4.MP4_CODEC_MP3)
				hasAudio = true
			}
			stop = muxer.Write(audio, frame, uint64(pts), uint64(dts))
		}
	}
	buf := make([]byte, 64<<10)
	for stop == nil {
		n, err := r.Read(buf)
		if n > 0 {
			if reader.Input(buf[:n]) != nil {
				return nil, errors.New("invalid FLV stream")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("archive read failed")
		}
	}
	switch {
	case stop == nil:
		return nil, errShort
	case stop != errDone:
		return nil, stop
	}
	if muxer.WriteTrailer() != nil {
		return nil, errors.New("cannot finish MP4")
	}
	return out.buf, nil
}

// memWriter is the io.WriteSeeker the MP4 muxer needs to patch box sizes.
type memWriter struct {
	buf []byte
	pos int
}

func (m *memWriter) Write(p []byte) (int, error) {
	if end := m.pos + len(p); end > len(m.buf) {
		m.buf = append(m.buf, make([]byte, end-len(m.buf))...)
	}
	copy(m.buf[m.pos:], p)
	m.pos += len(p)
	return len(p), nil
}

func (m *memWriter) Seek(offset int64, whence int) (int64, error) {
	pos := int64(m.pos)
	switch whence {
	case io.SeekStart:
		pos = offset
	case io.SeekCurrent:
		pos += offset
	case io.SeekEnd:
		pos = int64(len(m.buf)) + offset
	}
	if pos < 0 || pos > int64(len(m.buf)) {
		return 0, errors.New("seek out of range")
	}
	m.pos = int(pos)
	return pos, nil
}
