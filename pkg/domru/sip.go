package domru

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/moleus/domru/pkg/domru/models"
)

type SIPCredentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
	Realm    string `json:"realm"`
}

// integrationRequest keeps upstream payloads and credentials out of error messages.
func (w *APIWrapper) integrationRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, w.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("cannot create upstream request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "domru")
	res, err := w.authClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		return nil, fmt.Errorf("upstream HTTP %d", res.StatusCode)
	}
	return res, nil
}

func (w *APIWrapper) SIPDevice(ctx context.Context, place, control int, installation string) (SIPCredentials, string, error) {
	base := fmt.Sprintf("/rest/v1/places/%d/accesscontrols", place)
	res, err := w.integrationRequest(ctx, http.MethodGet, base, nil)
	if err != nil {
		return SIPCredentials{}, "", err
	}
	var controls models.AccessControlsResponse
	err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&controls)
	res.Body.Close()
	if err != nil {
		return SIPCredentials{}, "", fmt.Errorf("invalid access controls response")
	}
	name := ""
	found := false
	for _, ac := range controls.Data {
		if ac.ID == control {
			found = true
			name = ac.Name
			break
		}
	}
	if !found {
		return SIPCredentials{}, "", fmt.Errorf("configured intercom is not in accesscontrols")
	}
	res, err = w.integrationRequest(ctx, http.MethodPost, fmt.Sprintf("%s/%d/sipdevices", base, control), map[string]string{"installationId": installation})
	if err != nil {
		return SIPCredentials{}, "", err
	}
	defer res.Body.Close()
	var payload struct {
		Data SIPCredentials `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&payload) != nil {
		return SIPCredentials{}, "", fmt.Errorf("invalid SIP credentials response")
	}
	c := payload.Data
	if c.Login == "" || c.Password == "" || c.Realm == "" {
		return SIPCredentials{}, "", fmt.Errorf("incomplete SIP credentials")
	}
	return c, name, nil
}

// IntercomSnapshot returns a native 1920x1080 frame of the intercom camera.
// The door's own /accesscontrols/{id}/snapshots endpoint only ever renders the
// 500x281 thumbnail: with width/height it upscales it (1920x1080 comes back as
// a blurry 1920x1079), so the frame is taken from the forpost camera endpoint
// keyed by externalCameraId, which grabs it from the stream. Falls back to the
// thumbnail path when the camera is unknown - a small photo beats none.
func (w *APIWrapper) IntercomSnapshot(ctx context.Context, place, control int) ([]byte, error) {
	path := fmt.Sprintf("/rest/v1/places/%d/accesscontrols/%d/snapshots?width=1920&height=1080", place, control)
	if camera, err := w.IntercomCameraID(ctx, place, control); err == nil {
		path = fmt.Sprintf("/rest/v1/forpost/cameras/%s/snapshots?width=1920&height=1080", url.PathEscape(camera))
	}
	res, err := w.integrationRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, (10<<20)+1))
	if err != nil || len(data) > 10<<20 || http.DetectContentType(data) != "image/jpeg" {
		return nil, fmt.Errorf("invalid snapshot")
	}
	return data, nil
}

func (w *APIWrapper) OpenIntercom(ctx context.Context, place, control int) error {
	res, err := w.integrationRequest(ctx, http.MethodPost, fmt.Sprintf("/rest/v1/places/%d/accesscontrols/%d/actions", place, control), map[string]string{"name": "accessControlOpen"})
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}

// IntercomCameraID returns the forpost camera of the configured intercom
// (`externalCameraId` in /accesscontrols); the cloud archive is keyed by it.
func (w *APIWrapper) IntercomCameraID(ctx context.Context, place, control int) (string, error) {
	// ponytail: one lock for the single configured intercom; split per control if it ever grows.
	w.cameraMu.Lock()
	defer w.cameraMu.Unlock()
	if id, ok := w.cameras[control]; ok {
		return id, nil
	}
	res, err := w.integrationRequest(ctx, http.MethodGet, fmt.Sprintf("/rest/v1/places/%d/accesscontrols", place), nil)
	if err != nil {
		return "", err
	}
	var controls models.AccessControlsResponse
	err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&controls)
	res.Body.Close()
	if err != nil {
		return "", fmt.Errorf("invalid access controls response")
	}
	for _, ac := range controls.Data {
		if ac.ID != control {
			continue
		}
		var id string
		switch v := ac.ExternalCameraId.(type) {
		case string:
			id = v
		case float64:
			id = strconv.FormatFloat(v, 'f', 0, 64)
		}
		if id != "" {
			if w.cameras == nil {
				w.cameras = map[int]string{}
			}
			w.cameras[control] = id
			return id, nil
		}
		return "", fmt.Errorf("intercom has no camera")
	}
	return "", fmt.Errorf("configured intercom is not in accesscontrols")
}
