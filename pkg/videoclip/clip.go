// Package videoclip cuts a short MP4 around an intercom call.
//
// Two sources share the muxer: Source plays the operator's cloud archive
// (a request with TS=<unix seconds> returns that moment as HTTP-FLV in real
// time; needs recording on the tariff) and Buffer keeps the last seconds of
// the live stream in memory. The FLV (H.264 + MP3) is remuxed into MP4 in
// memory so Telegram plays it inline; nothing touches the disk.
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
	errEmpty = errors.New("no video frames")
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

// frame is one demuxed FLV frame; at is the wall clock when it arrived.
type frame struct {
	cid      codec.CodecID
	data     []byte
	pts, dts uint32
	at       time.Time
	idr      bool // set by Buffer.push
}

// key reports an H.264 frame that starts a decodable sequence. The FLV
// demuxer prepends SPS/PPS to every IDR, so either NALU marks one.
func (f frame) key() bool {
	if f.cid != codec.CODECID_VIDEO_H264 {
		return false
	}
	key := false
	codec.SplitFrame(f.data, func(nalu []byte) bool {
		if len(nalu) > 0 {
			switch codec.H264NaluTypeWithoutStartCode(nalu) {
			case codec.H264_NAL_I_SLICE, codec.H264_NAL_SPS:
				key = true
			}
		}
		return !key
	})
	return key
}

// parseFLV demuxes r and hands every frame (with its own copy of the data)
// to on. It returns nil at end of stream, the error on returned to stop
// early, or a generic error for a broken stream.
func parseFLV(r io.Reader, on func(frame) error) error {
	reader := flv.CreateFlvReader()
	var stop error
	reader.OnFrame = func(cid codec.CodecID, data []byte, pts, dts uint32) {
		if stop != nil {
			return
		}
		stop = on(frame{cid: cid, data: append([]byte(nil), data...), pts: pts, dts: dts, at: time.Now()})
	}
	buf := make([]byte, 64<<10)
	for stop == nil {
		n, err := r.Read(buf)
		if n > 0 {
			if reader.Input(buf[:n]) != nil {
				return errors.New("invalid FLV stream")
			}
		}
		if err == io.EOF {
			return stop
		}
		if err != nil {
			return errors.New("stream read failed")
		}
	}
	return stop
}

// Remux reads an FLV stream and returns an MP4 with the first d of video
// (plus the MP3 audio in that window). Fewer than d of video is an error:
// the caller retries once the archive has caught up.
func Remux(r io.Reader, d time.Duration) ([]byte, error) {
	var frames []frame
	var first uint32
	hasVideo := false
	limit := uint32(d / time.Millisecond)
	err := parseFLV(r, func(f frame) error {
		if f.cid == codec.CODECID_VIDEO_H264 {
			if !hasVideo {
				if time.Duration(f.dts)*time.Millisecond > liveThreshold {
					return errLive
				}
				hasVideo, first = true, f.dts
			}
			if f.dts-first >= limit {
				return errDone
			}
		}
		frames = append(frames, f)
		return nil
	})
	switch {
	case err == nil:
		return nil, errShort
	case err != errDone:
		return nil, err
	}
	return mux(frames)
}

// mux writes frames into an MP4 starting at the first video frame; audio
// before it is dropped and timestamps are rebased to zero.
func mux(frames []frame) ([]byte, error) {
	var out memWriter
	muxer, err := mp4.CreateMp4Muxer(&out)
	if err != nil {
		return nil, errors.New("cannot create MP4 muxer")
	}
	var video, audio uint32
	var hasVideo, hasAudio bool
	var base uint32
	rel := func(ts uint32) uint64 {
		if ts < base {
			return 0
		}
		return uint64(ts - base)
	}
	for _, f := range frames {
		switch f.cid {
		case codec.CODECID_VIDEO_H264:
			if !hasVideo {
				video = muxer.AddVideoTrack(mp4.MP4_CODEC_H264)
				hasVideo, base = true, f.dts
			}
			err = muxer.Write(video, f.data, rel(f.pts), rel(f.dts))
		case codec.CODECID_AUDIO_MP3:
			if !hasVideo {
				continue // align audio with the first video frame
			}
			if !hasAudio {
				audio = muxer.AddAudioTrack(mp4.MP4_CODEC_MP3)
				hasAudio = true
			}
			err = muxer.Write(audio, f.data, rel(f.pts), rel(f.dts))
		}
		if err != nil {
			return nil, errors.New("cannot write MP4")
		}
	}
	if !hasVideo {
		return nil, errEmpty
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
