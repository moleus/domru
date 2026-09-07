package controllers

import (
	"bytes"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	pagemodels "github.com/moleus/domru/cmd/models"
	"github.com/moleus/domru/pkg/domru"
	domrumodels "github.com/moleus/domru/pkg/domru/models"
	"github.com/stretchr/testify/require"
)

const cameraTestBase = "http://localhost:8080"

func TestCameraCardsMatchAcrossPlacesAndKeepStandaloneCameras(t *testing.T) {
	places := domrumodels.PlacesResponse{Data: []domrumodels.Data{
		{Place: domrumodels.Place{ID: 10, AccessControls: []domrumodels.AccessControl{
			{ID: 11, Name: "First door", ForpostGroupId: "101", AllowOpen: true, AllowVideo: true, PreviewAvailable: true},
			{ID: 12, Name: "Unmatched door", AllowVideo: true},
		}}},
		{Place: domrumodels.Place{ID: 20, AccessControls: []domrumodels.AccessControl{
			{ID: 21, Name: "Second door", ExternalCameraId: "202", AllowVideo: true, AllowSlideshow: true},
		}}},
	}}
	cameras := domrumodels.CamerasResponse{Data: []domrumodels.Camera{
		{ID: 202, IsActive: 1},
		{ID: 303, Name: "Standalone", IsActive: 1},
		{ID: 201, IsActive: 1, ParentGroups: []domrumodels.ParentGroup{{ID: 101}}},
	}}
	cards := buildCameraCards(cameraTestBase, places, cameras, nil)
	require.Len(t, cards, 4)
	require.Equal(t, 201, cards[0].ID)
	require.Equal(t, cameraTestBase+"/stream/201", cards[0].StreamURL)
	require.Equal(t, cameraTestBase+"/rest/v1/places/10/accesscontrols/11/actions", cards[0].OpenDoorURL)
	require.Zero(t, cards[1].ID, "an unmatched door must not borrow a camera by array position")
	require.Empty(t, cards[1].StreamURL)
	require.Equal(t, 202, cards[2].ID)
	require.Empty(t, cards[2].OpenDoorURL)
	require.Equal(t, 303, cards[3].ID)
	require.Contains(t, cards[3].SnapshotURL, "/forpost/cameras/303/snapshots")
	require.Empty(t, cards[3].OpenDoorURL)
}

func TestNeighborCameraEntitlements(t *testing.T) {
	for _, tc := range []struct {
		name         string
		activated    bool
		video        bool
		preview      bool
		id           string
		wantStream   bool
		wantSnapshot bool
	}{
		{name: "unpaid with advertised capabilities", video: true, preview: true, id: "301"},
		{name: "unpaid null ID", video: true, preview: true},
		{name: "paid", activated: true, video: true, preview: true, id: "301", wantStream: true, wantSnapshot: true},
		{name: "paid without video permission", activated: true, preview: true, id: "301", wantSnapshot: true},
		{name: "paid missing camera ID", activated: true, video: true, preview: true, wantSnapshot: true},
		{name: "paid invalid camera ID", activated: true, video: true, id: "../actions"},
		{name: "paid without preview", activated: true, video: true, id: "301", wantStream: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			places := domrumodels.PlacesResponse{Data: []domrumodels.Data{{Place: domrumodels.Place{ID: 10}}}}
			sections := map[int]domrumodels.ScreenSectionsResponse{10: {Sections: []domrumodels.ScreenSection{
				{Type: "ACCESS_CONTROL_CAMERA", Title: "Соседний подъезд", Entities: []domrumodels.AccessControlCamera{
					{AccessControlID: 31, Name: "Neighbor", ServiceActivated: tc.activated, AllowVideo: tc.video, PreviewAvailable: tc.preview, ExternalCameraID: tc.id},
				}},
			}}}
			cards := buildCameraCards(cameraTestBase, places, domrumodels.CamerasResponse{}, sections)
			require.Len(t, cards, 1)
			require.Equal(t, tc.wantStream, cards[0].StreamURL != "")
			require.Equal(t, tc.wantSnapshot, cards[0].SnapshotURL != "")
			require.Empty(t, cards[0].OpenDoorURL, "a neighboring camera never grants door access")
			if !tc.activated {
				require.Equal(t, "Требуется подписка Pro", cards[0].Status)
			}
			if tc.wantStream {
				require.Equal(t, cameraTestBase+"/stream/301", cards[0].StreamURL)
			}
			if tc.wantSnapshot {
				require.Contains(t, cards[0].SnapshotURL, "/places/10/accesscontrols/31/snapshots?")
			}
		})
	}
}

func TestCameraCardsDeduplicateSourcesAndIgnoreOtherSections(t *testing.T) {
	place := domrumodels.Data{Place: domrumodels.Place{ID: 10, AccessControls: []domrumodels.AccessControl{
		{ID: 11, ExternalCameraId: float64(201), AllowVideo: true},
	}}}
	sections := map[int]domrumodels.ScreenSectionsResponse{10: {Sections: []domrumodels.ScreenSection{
		{Type: "OTHER", Entities: []domrumodels.AccessControlCamera{{AccessControlID: 99}}},
		{Type: "ACCESS_CONTROL_CAMERA", Entities: []domrumodels.AccessControlCamera{
			{AccessControlID: 11, ExternalCameraID: "201", ServiceActivated: true, AllowVideo: true},
			{AccessControlID: 31, ExternalCameraID: "301", ServiceActivated: true, AllowVideo: true},
			{AccessControlID: 31, ExternalCameraID: "301", ServiceActivated: true, AllowVideo: true},
		}},
	}}}
	cards := buildCameraCards(cameraTestBase, domrumodels.PlacesResponse{Data: []domrumodels.Data{place, place}},
		domrumodels.CamerasResponse{Data: []domrumodels.Camera{{ID: 201}, {ID: 301}}}, sections)
	require.Len(t, cards, 2)
	require.Equal(t, 201, cards[0].ID)
	require.Equal(t, 301, cards[1].ID)
}

type cameraHTTPClient func(*http.Request) (*http.Response, error)

func (f cameraHTTPClient) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestHomePageLoadsSectionsAndPreservesCamerasOnFailure(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusForbidden, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var sectionCalls int
			client := cameraHTTPClient(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, r.Method)
				code := http.StatusOK
				var body string
				switch r.URL.Path {
				case "/rest/v1/forpost/cameras":
					body = `{"data":[{"ID":201,"IsActive":1,"ParentGroups":[{"ID":101}]}]}`
				case "/rest/v1/subscriberplaces":
					body = `{"data":[{"place":{"id":10,"accessControls":[{"id":11,"name":"Own","forpostGroupId":"101","allowOpen":true,"allowVideo":true,"previewAvailable":true}]}},{"place":{"id":20}}]}`
				case "/rest/v1/places/10/accesscontrols":
					body = `{"data":[{"id":11,"name":"Own","externalCameraId":"201","allowOpen":true,"allowVideo":true,"previewAvailable":true}]}`
				case "/rest/v1/places/20/accesscontrols":
					body = `{"data":[]}`
				case "/rest/v1/subscribers/profiles":
					body = `{}`
				case "/rest/v1/places/10/screen-sections", "/rest/v1/places/20/screen-sections":
					sectionCalls++
					code = status
					body = `{"errorCode":"UNAVAILABLE"}`
					if status == http.StatusOK {
						body = `{"sections":[{"type":"ACCESS_CONTROL_CAMERA","title":"Соседний подъезд","entities":[{"accessControlId":31,"name":"Neighbor","serviceActivated":true,"allowVideo":true,"previewAvailable":true,"externalCameraId":"301"},{"accessControlId":32,"name":"Locked","serviceActivated":false,"allowVideo":true,"externalCameraId":null}]}]}`
					}
				default:
					return nil, errors.New("unexpected endpoint: " + r.URL.Path)
				}
				return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})
			handler := &Handler{domruAPI: domru.NewDomruAPI(client)}
			data, err := handler.prepareHomePageData(httptest.NewRequest(http.MethodGet, cameraTestBase+"/pages/home.html", nil))
			require.NoError(t, err)
			require.Equal(t, 2, sectionCalls, "load sections for every account place")
			require.NotEmpty(t, data.CameraCards)
			require.Equal(t, 201, data.CameraCards[0].ID)
			require.NotEmpty(t, data.CameraCards[0].StreamURL)
			if status == http.StatusOK {
				require.Greater(t, len(data.CameraCards), 1)
				require.Equal(t, 301, data.CameraCards[1].ID)
			}
			if status == http.StatusOK || status == http.StatusNotFound {
				require.Empty(t, data.LoginError)
			} else {
				require.NotEmpty(t, data.LoginError)
			}
			html := renderCameraTestPage(t, data)
			require.Contains(t, html, "/stream/201", "partial errors must not hide working cameras")
		})
	}
}

func renderCameraTestPage(t *testing.T, data pagemodels.HomePageData) string {
	t.Helper()
	source, err := os.ReadFile("../../templates/home.html.tmpl")
	require.NoError(t, err)
	tmpl, err := template.New("home").Funcs(getTemplateFunctions()).Parse(string(source))
	require.NoError(t, err)
	var buffer bytes.Buffer
	require.NoError(t, tmpl.Execute(&buffer, data))
	return buffer.String()
}

func TestHomeTemplateMediaPermissionsAndEscaping(t *testing.T) {
	locked := pagemodels.CameraCard{Name: "<script>alert(1)</script>", Status: "Требуется подписка Pro"}
	html := renderCameraTestPage(t, pagemodels.HomePageData{CameraCards: []pagemodels.CameraCard{locked}})
	require.Contains(t, html, "Требуется подписка Pro")
	require.NotContains(t, html, "<img")
	require.NotContains(t, html, "<button")
	require.NotContains(t, html, "<pre>")
	require.NotContains(t, html, "<script>alert(1)</script>")
	require.Contains(t, html, "&lt;script&gt;")

	html = renderCameraTestPage(t, pagemodels.HomePageData{CameraCards: []pagemodels.CameraCard{
		{ID: 301, Name: "Neighbor", ConfigName: "domofon_10_31", SnapshotURL: cameraTestBase + "/snapshot", StreamURL: cameraTestBase + "/stream/301"},
	}})
	require.Contains(t, html, "<img")
	require.Contains(t, html, `href="http://localhost:8080/stream/301"`)
	require.Contains(t, html, "still_image_url:")
	require.Contains(t, html, "stream_source:")
	require.NotContains(t, html, "<button")
	require.NotContains(t, html, "rest_command:")
}

func TestHomeIgnoresLegacyNeighborDoorGrants(t *testing.T) {
	for _, controlsStatus := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(controlsStatus), func(t *testing.T) {
			client := cameraHTTPClient(func(r *http.Request) (*http.Response, error) {
				status := http.StatusOK
				var body string
				switch r.URL.Path {
				case "/rest/v1/forpost/cameras":
					body = `{"data":[{"ID":201,"IsActive":1},{"ID":301,"IsActive":1}]}`
				case "/rest/v1/subscriberplaces":
					body = `{"data":[{"place":{"id":10,"accessControls":[{"id":31,"externalCameraId":"301","allowOpen":true,"allowVideo":true},{"id":11,"externalCameraId":"201","allowOpen":true,"allowVideo":true}]}}]}`
				case "/rest/v1/places/10/accesscontrols":
					status = controlsStatus
					body = `{"data":[{"id":11,"externalCameraId":"201","allowOpen":true,"allowVideo":true}]}`
				case "/rest/v1/places/10/screen-sections":
					body = `{"sections":[{"type":"ACCESS_CONTROL_CAMERA","title":"Соседний подъезд","entities":[{"accessControlId":31,"name":"Neighbor","serviceActivated":false,"allowVideo":true,"externalCameraId":"301"}]}]}`
				case "/rest/v1/subscribers/profiles":
					body = `{}`
				default:
					return nil, errors.New("unexpected endpoint: " + r.URL.Path)
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})
			handler := &Handler{domruAPI: domru.NewDomruAPI(client)}
			data, err := handler.prepareHomePageData(httptest.NewRequest(http.MethodGet, cameraTestBase, nil))
			require.NoError(t, err)
			require.Len(t, data.CameraCards, 2)
			require.Equal(t, 201, data.CameraCards[0].ID)
			require.NotEmpty(t, data.CameraCards[0].StreamURL)
			require.Equal(t, controlsStatus == http.StatusOK, data.CameraCards[0].OpenDoorURL != "")
			require.Equal(t, "Соседний подъезд", data.CameraCards[1].Section)
			require.Empty(t, data.CameraCards[1].OpenDoorURL)
			require.Empty(t, data.CameraCards[1].StreamURL, "legacy lists cannot override an inactive entitlement")
			require.Empty(t, data.CameraCards[1].SnapshotURL)
		})
	}
}
