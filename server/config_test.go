package server_test

import (
	"github.com/stretchr/testify/assert"
	"heckel.io/ntfy/v2/server"
	"heckel.io/ntfy/v2/user"
	"testing"
)

func TestConfig_New(t *testing.T) {
	c := server.NewConfig()
	assert.Equal(t, ":80", c.ListenHTTP)
	assert.Equal(t, server.DefaultKeepaliveInterval, c.KeepaliveInterval)
}

func TestConfig_HashExcludesSecrets(t *testing.T) {
	// The config hash is served to browsers (ConfigHash, for webapp change detection), so
	// secret material must not feed it: a weak secret would otherwise be offline-brute-forceable
	// against a publicly visible hash.
	conf1 := server.NewConfig()
	conf2 := server.NewConfig()
	conf2.StripeSecretKey = "sk_live_topsecret"
	conf2.StripeWebhookKey = "whsec_topsecret"
	conf2.TwilioAuthToken = "twilio-auth-token"
	conf2.UpstreamAccessToken = "tk_upstream"
	conf2.WebPushPrivateKey = "web-push-private-key"
	conf2.SMTPSenderPass = "hunter2"
	conf2.AuthUsers = []*user.User{{Name: "phil", Hash: "$2a$10$somebcrypthash"}}
	conf2.AuthTokens = map[string][]*user.Token{"phil": {{Value: "tk_secrettoken"}}}
	assert.Equal(t, conf1.Hash(), conf2.Hash())
	// Non-secret fields must still change the hash
	conf3 := server.NewConfig()
	conf3.BaseURL = "https://ntfy.example.com"
	assert.NotEqual(t, conf1.Hash(), conf3.Hash())
}
