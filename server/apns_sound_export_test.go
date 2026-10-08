//go:build !nofirebase

package server

import (
	"encoding/json"
	"fmt"
	"heckel.io/ntfy/v2/model"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExportAPNSPayloadsForSim writes simctl-compatible .apns files from the
// real server toFirebaseMessage path so we can verify sound on the simulator.
func TestExportAPNSPayloadsForSim(t *testing.T) {
	outDir := os.Getenv("APNS_EXPORT_DIR")
	if outDir == "" {
		t.Skip("set APNS_EXPORT_DIR to export payloads")
	}
	require.NoError(t, os.MkdirAll(outDir, 0o755))

	type caseDef struct {
		name     string
		build    func() *model.Message
		auther   *testAuther
		wantKey  string // "sound" or "critical" or "none"
	}
	cases := []caseDef{
		{
			name: "normal_default_sound",
			build: func() *model.Message {
				m := model.NewDefaultMessage("probe-topic", "normal message with default sound")
				m.Title = "ntfy #1562 normal"
				m.Priority = 3
				return m
			},
			auther:  &testAuther{Allow: true},
			wantKey: "sound",
		},
		{
			name: "priority5_max_sound",
			build: func() *model.Message {
				m := model.NewDefaultMessage("probe-topic", "max priority message")
				m.Title = "ntfy #1562 max"
				m.Priority = 5
				return m
			},
			auther:  &testAuther{Allow: true},
			wantKey: "sound",
		},
		{
			name: "priority1_silent",
			build: func() *model.Message {
				m := model.NewDefaultMessage("probe-topic", "min priority silent")
				m.Title = "ntfy #1562 min"
				m.Priority = 1
				return m
			},
			auther:  &testAuther{Allow: true},
			wantKey: "none",
		},
		{
			name: "reserved_topic_poll_request_sound",
			build: func() *model.Message {
				// Auth denied → poll_request "New message" — was silent before the fix
				m := model.NewDefaultMessage("reserved-topic", "secret body")
				m.Title = "secret title"
				m.Priority = 3
				return m
			},
			auther:  &testAuther{Allow: false},
			wantKey: "sound",
		},
		{
			name: "pre_fix_shape_no_sound_control",
			build: func() *model.Message {
				// Used only as control comparison; we strip sound after export in the harness
				m := model.NewDefaultMessage("probe-topic", "control without sound key")
				m.Title = "control silent"
				m.Priority = 3
				return m
			},
			auther:  &testAuther{Allow: true},
			wantKey: "sound",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.build()
			fbm, err := toFirebaseMessage(m, tc.auther)
			require.NoError(t, err)
			require.NotNil(t, fbm.APNS)
			require.NotNil(t, fbm.APNS.Payload)
			require.NotNil(t, fbm.APNS.Payload.Aps)

			switch tc.wantKey {
			case "sound":
				require.Equal(t, "default", fbm.APNS.Payload.Aps.Sound)
			case "critical":
				require.NotNil(t, fbm.APNS.Payload.Aps.CriticalSound)
			case "none":
				require.Empty(t, fbm.APNS.Payload.Aps.Sound)
				require.Nil(t, fbm.APNS.Payload.Aps.CriticalSound)
			}

			// Build a simctl-compatible payload mirroring FCM→APNS shape
			aps := map[string]any{
				"mutable-content": 1,
				"alert": map[string]any{
					"title": fbm.APNS.Payload.Aps.Alert.Title,
					"body":  fbm.APNS.Payload.Aps.Alert.Body,
				},
			}
			if cs := fbm.APNS.Payload.Aps.CriticalSound; cs != nil {
				// APNS critical sound object (takes precedence over Sound string)
				aps["sound"] = map[string]any{
					"critical": 1,
					"name":     cs.Name,
					"volume":   cs.Volume,
				}
			} else if fbm.APNS.Payload.Aps.Sound != "" {
				aps["sound"] = fbm.APNS.Payload.Aps.Sound
			}
			payload := map[string]any{
				"Simulator Target Bundle": "com.ntfyfix.NotifSoundProbe",
				"aps":                     aps,
			}
			// include custom data keys like FCM does
			for k, v := range fbm.APNS.Payload.CustomData {
				payload[k] = v
			}
			b, err := json.MarshalIndent(payload, "", "  ")
			require.NoError(t, err)
			path := filepath.Join(outDir, tc.name+".apns")
			require.NoError(t, os.WriteFile(path, b, 0o644))
			fmt.Println("wrote", path)
		})
	}
}
