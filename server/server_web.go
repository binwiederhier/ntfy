package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"heckel.io/ntfy/v2/util"
)

// handleWebApp serves the embedded web app's index for client-side (SPA) routes that the
// browser router resolves, so the app shell loads and the client-side router takes over.
func (s *Server) handleWebApp(w http.ResponseWriter, r *http.Request, v *visitor) error {
	r.URL.Path = webAppIndex
	return s.handleStatic(w, r, v)
}

// handleWebAppNoIndex serves the web app index for the magic-link landing pages, whose path
// carries a one-time token. The response is marked no-referrer (so the token can't leak to third
// parties via the Referer header) and noindex (so it never gets indexed).
func (s *Server) handleWebAppNoIndex(w http.ResponseWriter, r *http.Request, v *visitor) error {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex")
	return s.handleWebApp(w, r, v)
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request, _ *visitor) error {
	w.Header().Set("Cache-Control", "no-cache")
	return s.writeJSON(w, s.configResponse())
}

func (s *Server) handleWebConfig(w http.ResponseWriter, _ *http.Request, _ *visitor) error {
	b, err := json.MarshalIndent(s.configResponse(), "", "  ")
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	_, err = io.WriteString(w, fmt.Sprintf("// Generated server configuration\nvar config = %s;\n", string(b)))
	return err
}

// handleWebManifest serves the web app manifest for the progressive web app (PWA)
func (s *Server) handleWebManifest(w http.ResponseWriter, _ *http.Request, _ *visitor) error {
	response := &webManifestResponse{
		Name:            "ntfy",
		Description:     "ntfy lets you send push notifications via scripts from any computer or phone",
		ShortName:       "ntfy",
		Scope:           "/",
		StartURL:        s.config.WebRoot,
		Display:         "standalone",
		BackgroundColor: "#ffffff",
		ThemeColor:      "#317f6f",
		Icons: []*webManifestIcon{
			{SRC: "/static/images/pwa-192x192.png", Sizes: "192x192", Type: "image/png"},
			{SRC: "/static/images/pwa-512x512.png", Sizes: "512x512", Type: "image/png"},
		},
	}
	return s.writeJSONWithContentType(w, response, "application/manifest+json")
}

// handleStatic returns all static resources (excluding the docs), including the web app
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	r.URL.Path = webSiteDir + r.URL.Path
	util.Gzip(http.FileServer(http.FS(webFsCached))).ServeHTTP(w, r)
	return nil
}

// handleDocs returns static resources related to the docs
func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	util.Gzip(http.FileServer(http.FS(docsStaticCached))).ServeHTTP(w, r)
	return nil
}

func (s *Server) configResponse() *apiConfigResponse {
	return &apiConfigResponse{
		BaseURL:             "", // Will translate to window.location.origin
		AppRoot:             s.config.WebRoot,
		EnableLogin:         s.config.EnableLogin,
		RequireLogin:        s.config.RequireLogin,
		EnableSignup:        s.config.EnableSignup,
		EnablePayments:      s.config.StripeSecretKey != "",
		EnableCalls:         s.config.TwilioAccount != "",
		EnableEmails:        s.config.SMTPSenderFrom != "",
		EnableResetPassword: s.config.SMTPSenderFrom != "" && s.config.BaseURL != "", // Reset links need SMTP + an absolute base-url
		EnableReservations:  s.config.EnableReservations,
		EnableWebPush:       s.config.WebPushPublicKey != "",
		BillingContact:      s.config.BillingContact,
		WebPushPublicKey:    s.config.WebPushPublicKey,
		DisallowedTopics:    s.config.DisallowedTopics,
		ConfigHash:          s.config.Hash(),
		AuthLogoutURL:       s.config.AuthLogoutURL,
	}
}
