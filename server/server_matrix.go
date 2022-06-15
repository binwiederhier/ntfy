package server

import (
	"encoding/json"
	"heckel.io/ntfy/log"
	"io"
	"net/http"
)

const (
	matrixPushkeyHeader = "X-Matrix-Pushkey"
)

type matrixMessage struct {
	Notification *matrixNotification `json:"notification"`
}

type matrixNotification struct {
	Devices []*matrixDevice `json:"devices"`
}

type matrixDevice struct {
	PushKey string `json:"pushkey"`
}

type matrixResponse struct {
	Rejected []string `json:"rejected"`
}

func handleMatrixDiscovery(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	_, err := io.WriteString(w, `{"unifiedpush":{"gateway":"matrix"}}`+"\n")
	return err
}

func writeMatrixError(w http.ResponseWriter, pushKey string, err error) error {
	log.Debug("Matrix message with push key %s rejected: %s", pushKey, err.Error())
	response := &matrixResponse{
		Rejected: []string{pushKey},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func writeMatrixSuccess(w http.ResponseWriter) error {
	response := &matrixResponse{
		Rejected: make([]string, 0),
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}
