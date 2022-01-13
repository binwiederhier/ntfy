package server

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMaybePeakAttachmentURL_Success(t *testing.T) {
	m := &message{
		Attachment: &attachment{
			URL: "https://ntfy.sh/static/img/ntfy.png",
		},
	}
	require.Nil(t, maybePeakAttachmentURL(m))
	require.Equal(t, "ntfy.png", m.Attachment.Name)
	require.Equal(t, int64(3627), m.Attachment.Size)
	require.Equal(t, "image/png", m.Attachment.Type)
	require.Equal(t, int64(0), m.Attachment.Expires)
}
