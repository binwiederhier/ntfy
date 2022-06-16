package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
	"io"
	"net/http"
	"strings"
)

const (
	matrixPushKeyHeader = "X-Matrix-Pushkey"
)

// matrixRequest represents a Matrix message, as it is sent to a Push Gateway (as per
// this spec: https://spec.matrix.org/v1.2/push-gateway-api/).
//
// From the message, we only require the "pushkey", as it represents our target topic URL.
// A message may look like this (excerpt):
//    {
//      "notification": {
//        "devices": [
//           {
//              "pushkey": "https://ntfy.sh/upDAHJKFFDFD?up=1",
//              ...
//           }
//        ]
//      }
//    }
type matrixRequest struct {
	Notification *struct {
		Devices []*struct {
			PushKey string `json:"pushkey"`
		} `json:"devices"`
	} `json:"notification"`
}

type matrixResponse struct {
	Rejected []string `json:"rejected"`
}

type errMatrix struct {
	pushKey string
	err     error
}

func (e errMatrix) Error() string {
	if e.err != nil {
		return fmt.Sprintf("message with push key %s rejected: %s", e.pushKey, e.err.Error())
	}
	return fmt.Sprintf("message with push key %s rejected", e.pushKey)
}

// newRequestFromMatrixJSON reads the request body as a Matrix JSON message, parses the "pushkey", and creates a new
// HTTP request that looks like a normal ntfy request from it.
//
// It basically converts a Matrix push gatewqy request:
//
//    POST /_matrix/push/v1/notify HTTP/1.1
//    { "notification": { "devices": [ { "pushkey": "https://ntfy.sh/upDAHJKFFDFD?up=1", ... } ] } }
//
// to a ntfy request, looking like this:
//
//    POST /upDAHJKFFDFD?up=1 HTTP/1.1
//    { "notification": { "devices": [ { "pushkey": "https://ntfy.sh/upDAHJKFFDFD?up=1", ... } ] } }
//
func newRequestFromMatrixJSON(r *http.Request, baseURL string, messageLimit int) (*http.Request, error) {
	if baseURL == "" {
		return nil, errHTTPInternalErrorMissingBaseURL
	}
	body, err := util.Peek(r.Body, messageLimit)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	var m matrixRequest
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		return nil, errHTTPBadRequestMatrixMessageInvalid
	} else if m.Notification == nil || len(m.Notification.Devices) == 0 || m.Notification.Devices[0].PushKey == "" {
		return nil, errHTTPBadRequestMatrixMessageInvalid
	}
	pushKey := m.Notification.Devices[0].PushKey
	if !strings.HasPrefix(pushKey, baseURL+"/") {
		return nil, &errMatrix{pushKey: pushKey, err: errHTTPBadRequestMatrixPushkeyBaseURLMismatch}
	}
	newRequest, err := http.NewRequest(http.MethodPost, pushKey, io.NopCloser(bytes.NewReader(body.PeekedBytes)))
	if err != nil {
		return nil, &errMatrix{pushKey: pushKey, err: err}
	}
	newRequest.Header.Set(matrixPushKeyHeader, pushKey)
	return newRequest, nil
}

func handleMatrixDiscovery(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	_, err := io.WriteString(w, `{"unifiedpush":{"gateway":"matrix"}}`+"\n")
	return err
}

func writeMatrixError(w http.ResponseWriter, r *http.Request, v *visitor, err *errMatrix) error {
	log.Debug("%s Matrix gateway error: %s", logHTTPPrefix(v, r), err.Error())
	return writeMatrixResponse(w, err.pushKey)
}

func writeMatrixSuccess(w http.ResponseWriter) error {
	return writeMatrixResponse(w, "")
}

func writeMatrixResponse(w http.ResponseWriter, rejectedPushKey string) error {
	rejected := make([]string, 0)
	if rejectedPushKey != "" {
		rejected = append(rejected, rejectedPushKey)
	}
	response := &matrixResponse{
		Rejected: rejected,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}
