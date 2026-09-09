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

func TestIntercomCameraIDFromAccessControls(t *testing.T) {
	body := `{"data":[{"id":2,"name":"Door","externalCameraId":"19410915"},{"id":3,"name":"NoCam","externalCameraId":null},{"id":4,"externalCameraId":77}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/rest/v1/places/1/accesscontrols", r.URL.Path)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	api := NewDomruAPI(srv.Client())
	api.baseURL = srv.URL
	id, err := api.IntercomCameraID(context.Background(), 1, 2)
	require.NoError(t, err)
	require.Equal(t, "19410915", id)
	id, err = api.IntercomCameraID(context.Background(), 1, 4)
	require.NoError(t, err)
	require.Equal(t, "77", id)
	_, err = api.IntercomCameraID(context.Background(), 1, 3)
	require.EqualError(t, err, "intercom has no camera")
	_, err = api.IntercomCameraID(context.Background(), 1, 99)
	require.Error(t, err)
}

func TestIntercomSnapshotUsesCameraEndpoint(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		switch r.URL.Path {
		case "/rest/v1/places/1/accesscontrols":
			w.Write([]byte(`{"data":[{"id":2,"externalCameraId":"1234567"},{"id":3}]}`))
		case "/rest/v1/forpost/cameras/1234567/snapshots", "/rest/v1/places/1/accesscontrols/3/snapshots":
			w.Write([]byte("\xff\xd8\xff\xe0jpeg"))
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	api := NewDomruAPI(srv.Client())
	api.baseURL = srv.URL

	for i := 0; i < 2; i++ {
		photo, err := api.IntercomSnapshot(context.Background(), 1, 2)
		require.NoError(t, err)
		require.NotEmpty(t, photo)
	}
	require.Equal(t, []string{
		"/rest/v1/places/1/accesscontrols",
		"/rest/v1/forpost/cameras/1234567/snapshots?width=1920&height=1080",
		"/rest/v1/forpost/cameras/1234567/snapshots?width=1920&height=1080",
	}, paths, "camera id is resolved once and the frame comes from the camera endpoint")

	// A door without a camera still gets the thumbnail instead of no photo.
	paths = nil
	_, err := api.IntercomSnapshot(context.Background(), 1, 3)
	require.NoError(t, err)
	require.Equal(t, "/rest/v1/places/1/accesscontrols/3/snapshots?width=1920&height=1080", paths[len(paths)-1])
}
