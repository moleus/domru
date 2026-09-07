package controllers

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/moleus/domru/cmd/models"
	"github.com/moleus/domru/pkg/domru/constants"
	domrumodels "github.com/moleus/domru/pkg/domru/models"
)

func cameraID(value interface{}) int {
	id, err := strconv.Atoi(fmt.Sprint(value))
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func accessControlCameraID(ac domrumodels.AccessControl, cameras []domrumodels.Camera) int {
	if id := cameraID(ac.ExternalCameraId); id != 0 {
		return id
	}
	for _, camera := range cameras {
		for _, group := range camera.ParentGroups {
			if strconv.Itoa(group.ID) == ac.ForpostGroupId {
				return camera.ID
			}
		}
	}
	return 0
}

func setCameraMedia(card *models.CameraCard, baseURL, snapshotURL string, allowVideo, allowSnapshot bool) {
	if allowVideo && card.ID > 0 {
		card.StreamURL = constants.GetCameraStreamUrl(baseURL, card.ID)
	}
	if allowSnapshot {
		card.SnapshotURL = snapshotURL
	}
	switch {
	case card.StreamURL != "":
		card.Status = "Доступна"
	case card.SnapshotURL != "":
		card.Status = "Доступен снимок; видеопоток недоступен"
	default:
		card.Status = "Просмотр недоступен"
	}
}

func buildCameraCards(baseURL string, places domrumodels.PlacesResponse, cameras domrumodels.CamerasResponse, sections map[int]domrumodels.ScreenSectionsResponse) []models.CameraCard {
	var cards []models.CameraCard
	seenCameras := make(map[int]bool)
	seenControls := make(map[[2]int]bool)
	sectionControls := make(map[[2]int]bool)
	for placeID, response := range sections {
		for _, section := range response.Sections {
			if section.Type == "ACCESS_CONTROL_CAMERA" {
				for _, camera := range section.Entities {
					sectionControls[[2]int{placeID, camera.AccessControlID}] = true
				}
			}
		}
	}

	for _, item := range places.Data {
		place := item.Place
		for _, ac := range place.AccessControls {
			key := [2]int{place.ID, ac.ID}
			if seenControls[key] || sectionControls[key] {
				continue
			}
			seenControls[key] = true
			id := accessControlCameraID(ac, cameras.Data)
			card := models.CameraCard{
				ID: id, Name: ac.Name, Section: "Мои домофоны",
				ConfigName: fmt.Sprintf("domofon_%d_%d", place.ID, ac.ID),
			}
			setCameraMedia(&card, baseURL, constants.GetSnapshotUrl(baseURL, place.ID, ac.ID),
				ac.AllowVideo, ac.AllowSlideshow || ac.PreviewAvailable)
			if ac.AllowOpen {
				card.OpenDoorURL = constants.GetOpenDoorUrl(baseURL, place.ID, ac.ID)
			}
			cards = append(cards, card)
			seenCameras[id] = true
		}
	}

	for _, item := range places.Data {
		placeID := item.Place.ID
		placeSections := append([]domrumodels.ScreenSection(nil), sections[placeID].Sections...)
		sort.SliceStable(placeSections, func(i, j int) bool { return placeSections[i].Order < placeSections[j].Order })
		for _, section := range placeSections {
			if section.Type != "ACCESS_CONTROL_CAMERA" {
				continue
			}
			for _, camera := range section.Entities {
				key := [2]int{placeID, camera.AccessControlID}
				if camera.AccessControlID <= 0 || seenControls[key] {
					continue
				}
				seenControls[key] = true
				id := cameraID(camera.ExternalCameraID)
				if id > 0 && seenCameras[id] {
					continue
				}
				card := models.CameraCard{
					ID: id, Name: camera.Name, Section: "Соседний подъезд",
					ConfigName: fmt.Sprintf("domofon_%d_%d", placeID, camera.AccessControlID),
					Status:     "Требуется подписка Pro",
				}
				if camera.ServiceActivated {
					snapshotURL := fmt.Sprintf("%s/rest/v1/places/%d/accesscontrols/%d/snapshots?width=320&height=180", baseURL, placeID, camera.AccessControlID)
					setCameraMedia(&card, baseURL, snapshotURL, camera.AllowVideo, camera.AllowSlideshow || camera.PreviewAvailable)
				}
				cards = append(cards, card)
				seenCameras[id] = true
			}
		}
	}

	// Keep standalone account cameras that are not associated with a door.
	for _, camera := range cameras.Data {
		if camera.ID <= 0 || seenCameras[camera.ID] {
			continue
		}
		card := models.CameraCard{
			ID: camera.ID, Name: camera.Name, Section: "Другие камеры",
			ConfigName: fmt.Sprintf("domru_camera_%d", camera.ID),
		}
		snapshotURL := fmt.Sprintf("%s/rest/v1/forpost/cameras/%d/snapshots?width=320&height=180", baseURL, camera.ID)
		setCameraMedia(&card, baseURL, snapshotURL, camera.IsActive == 1, camera.IsActive == 1)
		cards = append(cards, card)
		seenCameras[camera.ID] = true
	}
	return cards
}
