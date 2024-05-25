package controllers

import (
	"fmt"
	"log"
	"net/http"
)

func (h *Handler) StreamController(w http.ResponseWriter, r *http.Request) {
	log.Printf("StreamController: %s %s", r.Method, r.URL.Path)
	cameraId := r.PathValue("cameraId")
	if cameraId == "" {
		http.Error(w, "cameraId is required", http.StatusBadRequest)
		return
	}

	streamUrl, err := h.domruApi.GetStreamUrl(cameraId)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get stream url: %v", err), http.StatusInternalServerError)
		return
	}

	_, err = w.Write([]byte(streamUrl))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to write response: %v", err), http.StatusInternalServerError)
	}
}
