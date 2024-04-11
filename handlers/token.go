package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

func (h *Handler) Refresh(refreshToken *string) (string, string, error) {
	var (
		body   []byte
		err    error
	)

	url := API_REFRESH_SESSION
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", err
	}

	operator := strconv.Itoa(h.Config.Operator)

    rt := request.Header
	rt.Set("Content-Type", "application/json; charset=UTF-8")
	rt.Set("Operator", operator)
	rt.Set("Bearer", h.Config.RefreshToken)

	resp, err := h.Client.Do(request)
	if err != nil {
		return "", "", err
	}

	defer func() {
		err2 := resp.Body.Close()
		if err2 != nil {
			log.Println(err2)
		}
	}()

	if resp.StatusCode == 409 { // Conflict (tokent already expired)
		return "token can't be refreshed", "", nil
	}

	if body, err = io.ReadAll(resp.Body); err != nil {
		return "", "", err
	}

	var authResp ConfirmResponse
	if err = json.Unmarshal(body, &authResp); err != nil {
		return "", "", err
	}

	return authResp.AccessToken, authResp.RefreshToken, nil
}
