package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestServer_PublishMessageSizeBoundary(t *testing.T) {
	const limit = 32
	for _, attachments := range []bool{false, true} {
		for _, unknownLength := range []bool{false, true} {
			for _, tt := range []struct {
				name, body string
			}{
				{"below", strings.Repeat("a", limit-1)},
				{"exact_ascii", strings.Repeat("a", limit)},
				{"exact_utf8", strings.Repeat("a", limit-2) + "ł"},
				{"over", strings.Repeat("a", limit) + "Z"},
				{"invalid_utf8", strings.Repeat("a", limit-1) + "\xff"},
			} {
				t.Run(fmt.Sprintf("attachments=%t/unknown_length=%t/%s", attachments, unknownLength, tt.name), func(t *testing.T) {
					conf := newTestConfig(t, "")
					conf.MessageSizeLimit = limit
					conf.AttachmentCacheDir = ""
					if attachments {
						conf.AttachmentCacheDir = t.TempDir()
					}
					publisher := newTestServer(t, conf)
					var response *httptest.ResponseRecorder
					if unknownLength {
						req := httptest.NewRequest(http.MethodPost, "/mytopic", io.NopCloser(strings.NewReader(tt.body)))
						require.Equal(t, int64(-1), req.ContentLength)
						response = httptest.NewRecorder()
						publisher.handle(response, req)
					} else {
						response = request(t, publisher, http.MethodPost, "/mytopic", tt.body, nil)
					}
					isText := len(tt.body) <= limit && utf8.ValidString(tt.body)
					if !isText && !attachments {
						require.Equal(t, http.StatusBadRequest, response.Code)
						require.Contains(t, response.Body.String(), "attachments not allowed")
						return
					}
					require.Equal(t, http.StatusOK, response.Code)
					messages := toMessages(t, response.Body.String())
					require.Len(t, messages, 1)
					if isText {
						require.Equal(t, tt.body, messages[0].Message)
						require.Nil(t, messages[0].Attachment)
						return
					}
					require.NotNil(t, messages[0].Attachment)
					require.Equal(t, int64(len(tt.body)), messages[0].Attachment.Size)
					attachmentURL, err := url.Parse(messages[0].Attachment.URL)
					require.NoError(t, err)
					download := request(t, publisher, http.MethodGet, attachmentURL.Path, "", nil)
					require.Equal(t, http.StatusOK, download.Code)
					require.Equal(t, tt.body, download.Body.String())
				})
			}
		}
	}
}
