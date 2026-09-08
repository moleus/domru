package domru

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSIPUsesExplicitAccessControlsAndInstallation(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/rest/v1/places/1/accesscontrols":
			w.Write([]byte(`{"data":[{"id":2,"name":"Door"}]}`))
		case "/rest/v1/places/1/accesscontrols/2/sipdevices":
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "installation", body["installationId"])
			w.WriteHeader(201)
			w.Write([]byte(`{"data":{"login":"user","password":"secret","realm":"example.test"}}`))
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	api := NewDomruAPI(srv.Client())
	api.baseURL = srv.URL
	c, name, err := api.SIPDevice(context.Background(), 1, 2, "installation")
	require.NoError(t, err)
	require.Equal(t, "user", c.Login)
	require.Equal(t, "Door", name)
	require.Len(t, requests, 2)
	_, _, err = api.SIPDevice(context.Background(), 1, 99, "installation")
	require.Error(t, err)
	require.Len(t, requests, 3)
}
