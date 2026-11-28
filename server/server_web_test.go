package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServer_WebShareTarget(t *testing.T) {
	s := &Server{config: &Config{WebRoot: "/app"}}
	manifestRecorder := httptest.NewRecorder()
	require.NoError(t, s.handleWebManifest(manifestRecorder, httptest.NewRequest(http.MethodGet, webAppManifestPath, nil), nil))
	var manifest webManifestResponse
	require.NoError(t, json.Unmarshal(manifestRecorder.Body.Bytes(), &manifest))
	target := manifest.ShareTarget
	require.Equal(t, webAppSharePath, target.Action)
	require.Equal(t, http.MethodPost, target.Method)
	require.Equal(t, "application/x-www-form-urlencoded", target.Enctype)

	form := url.Values{
		target.Params.Title: {"Café + notes"},
		target.Params.Text:  {"Read this\nand pass it on"},
		target.Params.URL:   {"https://example.com/?a=1&b=2#details"},
		"topic":             {"not-selected"},
	}
	req := httptest.NewRequest(target.Method, target.Action+"?title=ignored", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", target.Enctype)
	rr := httptest.NewRecorder()
	require.NoError(t, s.handleWebShare(rr, req, nil))
	require.Equal(t, http.StatusSeeOther, rr.Code)
	require.Equal(t, "no-store", rr.Header().Get("Cache-Control"))
	path, encoded, found := strings.Cut(rr.Header().Get("Location"), "#share?")
	require.True(t, found)
	require.Equal(t, manifest.StartURL, path)
	draft, err := url.ParseQuery(encoded)
	require.NoError(t, err)
	require.Equal(t, url.Values{
		"title": {"Café + notes"},
		"text":  {"Read this\nand pass it on"},
		"url":   {"https://example.com/?a=1&b=2#details"},
	}, draft)
}
