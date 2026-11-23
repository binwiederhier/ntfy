package server_test

import (
	"github.com/stretchr/testify/assert"
	"heckel.io/ntfy/v2/server"
	"heckel.io/ntfy/v2/user"
	"net/netip"
	"testing"
)

func TestConfig_New(t *testing.T) {
	c := server.NewConfig()
	assert.Equal(t, []string{server.DefaultListenHTTP}, c.ListenHTTP)
	assert.Equal(t, server.DefaultKeepaliveInterval, c.KeepaliveInterval)
	assert.Equal(t, "", c.ListenAdminUnix) // Experimental, off unless configured
}

func TestConfig_HashExcludesSecrets(t *testing.T) {
	// The config hash is served to browsers (ConfigHash, for webapp change detection), so
	// secret material must not feed it: a weak secret would otherwise be offline-brute-forceable
	// against a publicly visible hash.
	conf1 := newConfigWithSecrets("one")
	conf2 := newConfigWithSecrets("two")
	assert.Equal(t, conf1.Hash(), conf2.Hash())
}

func TestConfig_HashSameAcrossClusterNodes(t *testing.T) {
	// Nodes behind the same load balancer must agree on the hash, or the web app asks for a
	// refresh whenever a poll lands on another node than the one that served the page.
	conf1 := newConfigForNode("node-1", "10.0.0.1")
	conf2 := newConfigForNode("node-2", "10.0.0.2")
	conf1.VisitorRequestExemptPrefixes = []netip.Prefix{netip.MustParsePrefix("1.1.1.1/32"), netip.MustParsePrefix("2.2.2.2/32")}
	conf2.VisitorRequestExemptPrefixes = []netip.Prefix{netip.MustParsePrefix("2.2.2.2/32"), netip.MustParsePrefix("1.1.1.1/32")} // DNS order differs
	assert.Equal(t, conf1.Hash(), conf2.Hash())
}

func TestConfig_HashChangesWithWebAppConfig(t *testing.T) {
	changes := map[string]func(c *server.Config){
		"build version":       func(c *server.Config) { c.BuildVersion = "2.99.0" },
		"build commit":        func(c *server.Config) { c.BuildCommit = "abcdef" },
		"build date":          func(c *server.Config) { c.BuildDate = "2026-11-23" },
		"web root":            func(c *server.Config) { c.WebRoot = "/app" },
		"login":               func(c *server.Config) { c.EnableLogin = true },
		"require login":       func(c *server.Config) { c.RequireLogin = true },
		"signup":              func(c *server.Config) { c.EnableSignup = true },
		"reservations":        func(c *server.Config) { c.EnableReservations = true },
		"payments":            func(c *server.Config) { c.StripeSecretKey = "sk_live_topsecret" },
		"calls":               func(c *server.Config) { c.TwilioAccount = "AC123" },
		"emails":              func(c *server.Config) { c.SMTPSenderFrom = "ntfy@example.com" },
		"billing contact":     func(c *server.Config) { c.BillingContact = "billing@example.com" },
		"web push public key": func(c *server.Config) { c.WebPushPublicKey = "public-key" },
		"disallowed topics":   func(c *server.Config) { c.DisallowedTopics = []string{"admin"} },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			conf := server.NewConfig()
			change(conf)
			assert.NotEqual(t, server.NewConfig().Hash(), conf.Hash())
		})
	}
}

func newConfigWithSecrets(secret string) *server.Config {
	conf := server.NewConfig()
	conf.DatabaseURL = "postgres://ntfy:" + secret + "@db.example.com/ntfy"
	conf.DatabaseReplicaURLs = []string{"postgres://ntfy:" + secret + "@replica.example.com/ntfy"}
	conf.AttachmentCacheDir = "s3://key:" + secret + "@bucket/attachments"
	conf.ClusterSecret = secret
	conf.StripeSecretKey = secret
	conf.StripeWebhookKey = secret
	conf.TwilioAuthToken = secret
	conf.UpstreamAccessToken = secret
	conf.WebPushPrivateKey = secret
	conf.SMTPSenderPass = secret
	conf.AuthUsers = []*user.User{{Name: "phil", Hash: secret}}
	conf.AuthTokens = map[string][]*user.Token{"phil": {{Value: secret}}}
	return conf
}

func newConfigForNode(nodeID, ip string) *server.Config {
	conf := server.NewConfig()
	conf.ClusterNodeID = nodeID
	conf.ClusterListen = ip + ":25000"
	conf.ClusterAdvertiseURL = "http://" + ip + ":25000"
	conf.ListenHTTP = []string{ip + ":25010"}
	conf.MetricsListenHTTP = ip + ":9090"
	return conf
}
