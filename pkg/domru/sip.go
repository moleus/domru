package domru

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

func (w *APIWrapper) IntercomSnapshot(ctx context.Context, place, control int) ([]byte, error) {
	res, err := w.integrationRequest(ctx, http.MethodGet, fmt.Sprintf("/rest/v1/places/%d/accesscontrols/%d/snapshots", place, control), nil)
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
