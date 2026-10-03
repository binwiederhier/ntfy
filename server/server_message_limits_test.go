package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServer_PublishConfiguredTitleAndTagsLimits(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		for _, limit := range []int{12, 2048} {
			for _, jsonBody := range []bool{false, true} {
				t.Run(fmt.Sprintf("limit=%d/json=%t", limit, jsonBody), func(t *testing.T) {
					c := newTestConfig(t, databaseURL)
					c.MessageTitleSizeLimit = limit
					c.MessageTagsSizeLimit = limit
					s := newTestServer(t, c)
					for _, tc := range []struct {
						name, title     string
						tags            []string
						code, errorCode int
					}{
						{"at_limits", strings.Repeat("t", limit), []string{strings.Repeat("a", limit/2), strings.Repeat("b", limit-limit/2)}, 200, 0},
						{"title_over", strings.Repeat("t", limit+1), nil, 400, 40057},
						{"tags_total_over", "ok", []string{strings.Repeat("a", limit/2), strings.Repeat("b", limit-limit/2+1)}, 400, 40058},
						{"multibyte_title", strings.Repeat("é", limit/2+1), nil, 400, 40057},
						{"multibyte_tags", "ok", []string{strings.Repeat("é", limit/2+1)}, 400, 40058},
					} {
						t.Run(tc.name, func(t *testing.T) {
							method, path, body := "PUT", "/limits", "message"
							headers := map[string]string{"Title": tc.title, "Tags": strings.Join(tc.tags, ",")}
							if jsonBody {
								data, err := json.Marshal(map[string]any{"topic": "limits", "message": body, "title": tc.title, "tags": tc.tags})
								require.NoError(t, err)
								method, path, body, headers = "POST", "/", string(data), nil
							}
							response := request(t, s, method, path, body, headers)
							require.Equal(t, tc.code, response.Code, response.Body.String())
							if tc.errorCode != 0 {
								require.Equal(t, tc.errorCode, toHTTPError(t, response.Body.String()).Code)
							} else {
								m := toMessage(t, response.Body.String())
								require.Equal(t, tc.title, m.Title)
								require.Equal(t, tc.tags, m.Tags)
							}
						})
					}
				})
			}
		}
	})
}
