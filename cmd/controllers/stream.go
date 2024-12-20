package controllers

import (
	"fmt"
	"net/http"
)

func (h *Handler) StreamController(w http.ResponseWriter, r *http.Request) {
	h.Logger.Debug("StreamController: %s %s", r.Method, r.URL.Path)
	cameraID := r.PathValue("cameraId")
	if cameraID == "" {
		http.Error(w, "cameraId is required", http.StatusBadRequest)
		return
	}

	streamURL, err := h.domruAPI.GetStreamURL(cameraID, r.URL.Query())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get stream url: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, streamURL, http.StatusFound)
}
