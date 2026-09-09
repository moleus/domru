package videoclip

import (
	"bytes"
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	flv "github.com/yapingcat/gomedia/go-flv"
)

// Real SPS/PPS of the Дом.ру stream (1080p); slices are junk, the muxer never decodes them.
var (
	sps, _ = hex.DecodeString("674d002a963540f0044fcb3701010102")
	pps, _ = hex.DecodeString("68ee3c80")
)

// fakeFLV renders `frames` H.264 frames at 25 fps starting at `base` ms, a
// keyframe every second.
func fakeFLV(t *testing.T, frames int, base uint32) []byte {
	var out bytes.Buffer
	w := flv.CreateFlvWriter(&out)
	if err := w.WriteFlvHeader(); err != nil {
		t.Fatal(err)
	}
	start := []byte{0, 0, 0, 1}
	for i := 0; i < frames; i++ {
		var frame []byte
		if i%25 == 0 {
			frame = append(frame, start...)
			frame = append(frame, sps...)
			frame = append(frame, start...)
			frame = append(frame, pps...)
			frame = append(frame, start...)
			frame = append(frame, 0x65, 0x88, 0x84, byte(i), 1, 2, 3)
		} else {
			frame = append(frame, start...)
			frame = append(frame, 0x41, 0x9a, byte(i), 4, 5, 6)
		}
		ts := base + uint32(i)*40
		if err := w.WriteH264(frame, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	return out.Bytes()
}

func TestRemuxCutsToDuration(t *testing.T) {
	mp4, err := Remux(bytes.NewReader(fakeFLV(t, 30*25, 383)), 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mp4[4:8], []byte("ftyp")) || !bytes.Contains(mp4, []byte("moov")) || !bytes.Contains(mp4, []byte("avc1")) {
		t.Fatalf("not an MP4: %x", mp4[:16])
	}
	// stsz (sample sizes) must hold exactly 20 s * 25 fps entries.
	i := bytes.Index(mp4, []byte("stsz"))
	if i < 0 {
		t.Fatal("no stsz")
	}
	count := uint32(mp4[i+12])<<24 | uint32(mp4[i+13])<<16 | uint32(mp4[i+14])<<8 | uint32(mp4[i+15])
	if count != 500 {
		t.Fatalf("samples = %d, want 500", count)
	}
}

func TestRemuxErrors(t *testing.T) {
	if _, err := Remux(bytes.NewReader(fakeFLV(t, 10*25, 0)), 20*time.Second); err != errShort {
		t.Fatalf("short stream: %v", err)
	}
	if _, err := Remux(bytes.NewReader(fakeFLV(t, 30*25, 433696720)), 20*time.Second); err != errLive {
		t.Fatalf("live fallback: %v", err)
	}
	if _, err := Remux(bytes.NewReader([]byte("HTTP/1.0 404 Not Found")), time.Second); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestClipRequestsArchiveByTS(t *testing.T) {
	stream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		_, _ = w.Write(fakeFLV(t, 5*25, 0))
	}))
	defer stream.Close()
	var gotCamera string
	var gotQuery url.Values
	cameraCalls := 0
	s := &Source{
		Camera: func(context.Context) (string, error) { cameraCalls++; return "19410915", nil },
		URL: func(camera string, q url.Values) (string, error) {
			gotCamera, gotQuery = camera, q
			return stream.URL + "/rtsp/x/secret-token", nil
		},
		Client: stream.Client(),
	}
	start := time.Unix(1757360000, 0)
	for i := 0; i < 2; i++ {
		if _, err := s.Clip(context.Background(), start, 2*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if gotCamera != "19410915" || gotQuery.Get("TS") != "1757360000" || gotQuery.Get("Format") != "H264" || gotQuery.Has("TZ") {
		t.Fatalf("query %v for camera %q", gotQuery, gotCamera)
	}
	if cameraCalls != 1 {
		t.Fatalf("camera resolved %d times", cameraCalls)
	}
	stream.Close()
	_, err := s.Clip(context.Background(), start, 2*time.Second)
	if err == nil || bytes.Contains([]byte(err.Error()), []byte("secret-token")) {
		t.Fatalf("error leaks URL or missing: %v", err)
	}
}
