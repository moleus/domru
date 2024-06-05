package controllers

import (
	"fmt"
	"net/http"
)

func (h *Handler) StreamController(w http.ResponseWriter, r *http.Request) {
	h.Logger.Debug("StreamController: %s %s", r.Method, r.URL.Path)
	cameraId := r.PathValue("cameraId")
	if cameraId == "" {
		http.Error(w, "cameraId is required", http.StatusBadRequest)
		return
	}

	streamUrl, err := h.domruApi.GetStreamUrl(cameraId, r.URL.Query())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get stream url: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, streamUrl, http.StatusFound)
}
