package server

import (
	"fmt"
	"heckel.io/ntfy/util"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"
)

const (
	peakAttachmentTimeout    = 2500 * time.Millisecond
	peakAttachmeantReadBytes = 128
)

func maybePeakAttachmentURL(m *message) error {
	return maybePeakAttachmentURLInternal(m, peakAttachmentTimeout)
}

func maybePeakAttachmentURLInternal(m *message, timeout time.Duration) error {
	if m.Attachment == nil || m.Attachment.URL == "" {
		return nil
	}
	client := http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableCompression: true, // Disable "Accept-Encoding: gzip", otherwise we won't get the Content-Length
			Proxy:              http.ProxyFromEnvironment,
		},
	}
	req, err := http.NewRequest(http.MethodGet, m.Attachment.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ntfy")
	resp, err := client.Do(req)
	if err != nil {
		return errHTTPBadRequestAttachmentURLPeakGeneral
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errHTTPBadRequestAttachmentURLPeakNon2xx
	}
	if size, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64); err == nil {
		m.Attachment.Size = size
	}
	m.Attachment.Type = resp.Header.Get("Content-Type")
	if m.Attachment.Type == "" || m.Attachment.Type == "application/octet-stream" {
		buf := make([]byte, peakAttachmeantReadBytes)
		io.ReadFull(resp.Body, buf) // Best effort: We don't care about the error
		m.Attachment.Type = http.DetectContentType(buf)
	}
	if m.Attachment.Name == "" {
		u, err := url.Parse(m.Attachment.URL)
		if err != nil {
			m.Attachment.Name = fmt.Sprintf("attachment%s", util.ExtensionByType(m.Attachment.Type))
		} else {
			m.Attachment.Name = path.Base(u.Path)
			if m.Attachment.Name == "." || m.Attachment.Name == "/" {
				m.Attachment.Name = fmt.Sprintf("attachment%s", util.ExtensionByType(m.Attachment.Type))
			}
		}
	}
	return nil
}
