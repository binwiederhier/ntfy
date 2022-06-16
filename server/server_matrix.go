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

// Matrix Push Gateway / UnifiedPush / ntfy integration:
//
// ntfy implements a Matrix Push Gateway (as defined in https://spec.matrix.org/v1.2/push-gateway-api/),
// in combination with UnifiedPush as the Provider Push Protocol (as defined in https://unifiedpush.org/developers/gateway/).
//
// In the picture below, ntfy is the Push Gateway (mostly in this file), as well as the Push Provider (ntfy's
// main functionality). UnifiedPush is the Provider Push Protocol, as implemented by the ntfy server and the
// ntfy Android app.
//
//                                    +--------------------+  +-------------------+
//                  Matrix HTTP      |                    |  |                   |
//             Notification Protocol |   App Developer    |  |   Device Vendor   |
//                                   |                    |  |                   |
//           +-------------------+   | +----------------+ |  | +---------------+ |
//           |                   |   | |                | |  | |               | |
//           | Matrix homeserver +----->  Push Gateway  +------> Push Provider | |
//           |                   |   | |                | |  | |               | |
//           +-^-----------------+   | +----------------+ |  | +----+----------+ |
//             |                     |                    |  |      |            |
//    Matrix   |                     |                    |  |      |            |
// Client/Server API  +              |                    |  |      |            |
//             |      |              +--------------------+  +-------------------+
//             |   +--+-+                                           |
//             |   |    <-------------------------------------------+
//             +---+    |
//                 |    |          Provider Push Protocol
//                 +----+
//
//         Mobile Device or Client
//

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
//
type matrixRequest struct {
	Notification *struct {
		Devices []*struct {
			PushKey string `json:"pushkey"`
		} `json:"devices"`
	} `json:"notification"`
}

// matrixResponse represents the response to a Matrix push gateway message, as defined
// in the spec (https://spec.matrix.org/v1.2/push-gateway-api/).
type matrixResponse struct {
	Rejected []string `json:"rejected"`
}

// errMatrix represents an error when handing Matrix gateway messages
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

const (
	// matrixPushKeyHeader is a header that's used internally to pass the Matrix push key (from the matrixRequest)
	// along with the request. The push key is only used if an error occurs down the line.
	matrixPushKeyHeader = "X-Matrix-Pushkey"
)

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
	pushKey := m.Notification.Devices[0].PushKey // We ignore other devices for now, see discussion in #316
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

// writeMatrixDiscoveryResponse writes the UnifiedPush Matrix Gateway Discovery response to the given http.ResponseWriter,
// as per the spec (https://unifiedpush.org/developers/gateway/).
func writeMatrixDiscoveryResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	_, err := io.WriteString(w, `{"unifiedpush":{"gateway":"matrix"}}`+"\n")
	return err
}

// writeMatrixError logs and writes the errMatrix to the given http.ResponseWriter as a matrixResponse
func writeMatrixError(w http.ResponseWriter, r *http.Request, v *visitor, err *errMatrix) error {
	log.Debug("%s Matrix gateway error: %s", logHTTPPrefix(v, r), err.Error())
	return writeMatrixResponse(w, err.pushKey)
}

// writeMatrixSuccess writes a successful matrixResponse (no rejected push key) to the given http.ResponseWriter
func writeMatrixSuccess(w http.ResponseWriter) error {
	return writeMatrixResponse(w, "")
}

// writeMatrixResponse writes a matrixResponse to the given http.ResponseWriter, as defined in
// the spec (https://spec.matrix.org/v1.2/push-gateway-api/)
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
