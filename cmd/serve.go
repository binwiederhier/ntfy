//go:build !noserver

package cmd

import (
	"errors"
	"fmt"
	"github.com/stripe/stripe-go/v74"
	"heckel.io/ntfy/user"
	"io/fs"
	"math"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"heckel.io/ntfy/log"

	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"heckel.io/ntfy/server"
	"heckel.io/ntfy/util"
)

func init() {
	commands = append(commands, cmdServe)
}

const (
	defaultServerConfigFile = "/etc/ntfy/server.yml"
)

var flagsServe = append(
	append([]cli.Flag{}, flagsDefault...),
	&cli.StringFlag{Name: "config", Aliases: []string{"c"}, EnvVars: []string{"NTFY_CONFIG_FILE"}, Value: defaultServerConfigFile, DefaultText: defaultServerConfigFile, Usage: "config file"},
	altsrc.NewStringFlag(&cli.StringFlag{Name: "base-url", Aliases: []string{"base_url", "B"}, EnvVars: []string{"NTFY_BASE_URL"}, Usage: "externally visible base URL for this host (e.g. https://ntfy.sh)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "listen-http", Aliases: []string{"listen_http", "l"}, EnvVars: []string{"NTFY_LISTEN_HTTP"}, Value: server.DefaultListenHTTP, Usage: "ip:port used as HTTP listen address"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "listen-https", Aliases: []string{"listen_https", "L"}, EnvVars: []string{"NTFY_LISTEN_HTTPS"}, Usage: "ip:port used as HTTPS listen address"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "listen-unix", Aliases: []string{"listen_unix", "U"}, EnvVars: []string{"NTFY_LISTEN_UNIX"}, Usage: "listen on unix socket path"}),
	altsrc.NewIntFlag(&cli.IntFlag{Name: "listen-unix-mode", Aliases: []string{"listen_unix_mode"}, EnvVars: []string{"NTFY_LISTEN_UNIX_MODE"}, DefaultText: "system default", Usage: "file permissions of unix socket, e.g. 0700"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "key-file", Aliases: []string{"key_file", "K"}, EnvVars: []string{"NTFY_KEY_FILE"}, Usage: "private key file, if listen-https is set"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "cert-file", Aliases: []string{"cert_file", "E"}, EnvVars: []string{"NTFY_CERT_FILE"}, Usage: "certificate file, if listen-https is set"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "firebase-key-file", Aliases: []string{"firebase_key_file", "F"}, EnvVars: []string{"NTFY_FIREBASE_KEY_FILE"}, Usage: "Firebase credentials file; if set additionally publish to FCM topic"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "cache-file", Aliases: []string{"cache_file", "C"}, EnvVars: []string{"NTFY_CACHE_FILE"}, Usage: "cache file used for message caching"}),
	altsrc.NewDurationFlag(&cli.DurationFlag{Name: "cache-duration", Aliases: []string{"cache_duration", "b"}, EnvVars: []string{"NTFY_CACHE_DURATION"}, Value: server.DefaultCacheDuration, Usage: "buffer messages for this time to allow `since` requests"}),
	altsrc.NewIntFlag(&cli.IntFlag{Name: "cache-batch-size", Aliases: []string{"cache_batch_size"}, EnvVars: []string{"NTFY_BATCH_SIZE"}, Usage: "max size of messages to batch together when writing to message cache (if zero, writes are synchronous)"}),
	altsrc.NewDurationFlag(&cli.DurationFlag{Name: "cache-batch-timeout", Aliases: []string{"cache_batch_timeout"}, EnvVars: []string{"NTFY_CACHE_BATCH_TIMEOUT"}, Usage: "timeout for batched async writes to the message cache (if zero, writes are synchronous)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "cache-startup-queries", Aliases: []string{"cache_startup_queries"}, EnvVars: []string{"NTFY_CACHE_STARTUP_QUERIES"}, Usage: "queries run when the cache database is initialized"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-file", Aliases: []string{"auth_file", "H"}, EnvVars: []string{"NTFY_AUTH_FILE"}, Usage: "auth database file used for access control"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-startup-queries", Aliases: []string{"auth_startup_queries"}, EnvVars: []string{"NTFY_AUTH_STARTUP_QUERIES"}, Usage: "queries run when the auth database is initialized"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-default-access", Aliases: []string{"auth_default_access", "p"}, EnvVars: []string{"NTFY_AUTH_DEFAULT_ACCESS"}, Value: "read-write", Usage: "default permissions if no matching entries in the auth database are found"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "attachment-cache-dir", Aliases: []string{"attachment_cache_dir"}, EnvVars: []string{"NTFY_ATTACHMENT_CACHE_DIR"}, Usage: "cache directory for attached files"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "attachment-total-size-limit", Aliases: []string{"attachment_total_size_limit", "A"}, EnvVars: []string{"NTFY_ATTACHMENT_TOTAL_SIZE_LIMIT"}, DefaultText: "5G", Usage: "limit of the on-disk attachment cache"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "attachment-file-size-limit", Aliases: []string{"attachment_file_size_limit", "Y"}, EnvVars: []string{"NTFY_ATTACHMENT_FILE_SIZE_LIMIT"}, DefaultText: "15M", Usage: "per-file attachment size limit (e.g. 300k, 2M, 100M)"}),
	altsrc.NewDurationFlag(&cli.DurationFlag{Name: "attachment-expiry-duration", Aliases: []string{"attachment_expiry_duration", "X"}, EnvVars: []string{"NTFY_ATTACHMENT_EXPIRY_DURATION"}, Value: server.DefaultAttachmentExpiryDuration, DefaultText: "3h", Usage: "duration after which uploaded attachments will be deleted (e.g. 3h, 20h)"}),
	altsrc.NewDurationFlag(&cli.DurationFlag{Name: "keepalive-interval", Aliases: []string{"keepalive_interval", "k"}, EnvVars: []string{"NTFY_KEEPALIVE_INTERVAL"}, Value: server.DefaultKeepaliveInterval, Usage: "interval of keepalive messages"}),
	altsrc.NewDurationFlag(&cli.DurationFlag{Name: "manager-interval", Aliases: []string{"manager_interval", "m"}, EnvVars: []string{"NTFY_MANAGER_INTERVAL"}, Value: server.DefaultManagerInterval, Usage: "interval of for message pruning and stats printing"}),
	altsrc.NewStringSliceFlag(&cli.StringSliceFlag{Name: "disallowed-topics", Aliases: []string{"disallowed_topics"}, EnvVars: []string{"NTFY_DISALLOWED_TOPICS"}, Usage: "topics that are not allowed to be used"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "web-root", Aliases: []string{"web_root"}, EnvVars: []string{"NTFY_WEB_ROOT"}, Value: "/", Usage: "sets root of the web app (e.g. /, or /app), or disables it (disable)"}),
	altsrc.NewBoolFlag(&cli.BoolFlag{Name: "enable-signup", Aliases: []string{"enable_signup"}, EnvVars: []string{"NTFY_ENABLE_SIGNUP"}, Value: false, Usage: "allows users to sign up via the web app, or API"}),
	altsrc.NewBoolFlag(&cli.BoolFlag{Name: "enable-login", Aliases: []string{"enable_login"}, EnvVars: []string{"NTFY_ENABLE_LOGIN"}, Value: false, Usage: "allows users to log in via the web app, or API"}),
	altsrc.NewBoolFlag(&cli.BoolFlag{Name: "enable-reservations", Aliases: []string{"enable_reservations"}, EnvVars: []string{"NTFY_ENABLE_RESERVATIONS"}, Value: false, Usage: "allows users to reserve topics (if their tier allows it)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "upstream-base-url", Aliases: []string{"upstream_base_url"}, EnvVars: []string{"NTFY_UPSTREAM_BASE_URL"}, Value: "", Usage: "forward poll request to an upstream server, this is needed for iOS push notifications for self-hosted servers"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "upstream-access-token", Aliases: []string{"upstream_access_token"}, EnvVars: []string{"NTFY_UPSTREAM_ACCESS_TOKEN"}, Value: "", Usage: "access token to use for the upstream server; needed only if upstream rate limits are exceeded or upstream server requires auth"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-sender-addr", Aliases: []string{"smtp_sender_addr"}, EnvVars: []string{"NTFY_SMTP_SENDER_ADDR"}, Usage: "SMTP server address (host:port) for outgoing emails"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-sender-user", Aliases: []string{"smtp_sender_user"}, EnvVars: []string{"NTFY_SMTP_SENDER_USER"}, Usage: "SMTP user (if e-mail sending is enabled)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-sender-pass", Aliases: []string{"smtp_sender_pass"}, EnvVars: []string{"NTFY_SMTP_SENDER_PASS"}, Usage: "SMTP password (if e-mail sending is enabled)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-sender-from", Aliases: []string{"smtp_sender_from"}, EnvVars: []string{"NTFY_SMTP_SENDER_FROM"}, Usage: "SMTP sender address (if e-mail sending is enabled)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-server-listen", Aliases: []string{"smtp_server_listen"}, EnvVars: []string{"NTFY_SMTP_SERVER_LISTEN"}, Usage: "SMTP server address (ip:port) for incoming emails, e.g. :25"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-server-domain", Aliases: []string{"smtp_server_domain"}, EnvVars: []string{"NTFY_SMTP_SERVER_DOMAIN"}, Usage: "SMTP domain for incoming e-mail, e.g. ntfy.sh"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "smtp-server-addr-prefix", Aliases: []string{"smtp_server_addr_prefix"}, EnvVars: []string{"NTFY_SMTP_SERVER_ADDR_PREFIX"}, Usage: "SMTP email address prefix for topics to prevent spam (e.g. 'ntfy-')"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "twilio-account", Aliases: []string{"twilio_account"}, EnvVars: []string{"NTFY_TWILIO_ACCOUNT"}, Usage: "Twilio account SID, used for phone calls, e.g. AC123..."}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "twilio-auth-token", Aliases: []string{"twilio_auth_token"}, EnvVars: []string{"NTFY_TWILIO_AUTH_TOKEN"}, Usage: "Twilio auth token"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "twilio-phone-number", Aliases: []string{"twilio_phone_number"}, EnvVars: []string{"NTFY_TWILIO_PHONE_NUMBER"}, Usage: "Twilio number to use for outgoing calls"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "twilio-verify-service", Aliases: []string{"twilio_verify_service"}, EnvVars: []string{"NTFY_TWILIO_VERIFY_SERVICE"}, Usage: "Twilio Verify service ID, used for phone number verification"}),
	altsrc.NewIntFlag(&cli.IntFlag{Name: "global-topic-limit", Aliases: []string{"global_topic_limit", "T"}, EnvVars: []string{"NTFY_GLOBAL_TOPIC_LIMIT"}, Value: server.DefaultTotalTopicLimit, Usage: "total number of topics allowed"}),
	altsrc.NewIntFlag(&cli.IntFlag{Name: "visitor-subscription-limit", Aliases: []string{"visitor_subscription_limit"}, EnvVars: []string{"NTFY_VISITOR_SUBSCRIPTION_LIMIT"}, Value: server.DefaultVisitorSubscriptionLimit, Usage: "number of subscriptions per visitor"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "visitor-attachment-total-size-limit", Aliases: []string{"visitor_attachment_total_size_limit"}, EnvVars: []string{"NTFY_VISITOR_ATTACHMENT_TOTAL_SIZE_LIMIT"}, Value: "100M", Usage: "total storage limit used for attachments per visitor"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "visitor-attachment-daily-bandwidth-limit", Aliases: []string{"visitor_attachment_daily_bandwidth_limit"}, EnvVars: []string{"NTFY_VISITOR_ATTACHMENT_DAILY_BANDWIDTH_LIMIT"}, Value: "500M", Usage: "total daily attachment download/upload bandwidth limit per visitor"}),
	altsrc.NewIntFlag(&cli.IntFlag{Name: "visitor-request-limit-burst", Aliases: []string{"visitor_request_limit_burst"}, EnvVars: []string{"NTFY_VISITOR_REQUEST_LIMIT_BURST"}, Value: server.DefaultVisitorRequestLimitBurst, Usage: "initial limit of requests per visitor"}),
	altsrc.NewDurationFlag(&cli.DurationFlag{Name: "visitor-request-limit-replenish", Aliases: []string{"visitor_request_limit_replenish"}, EnvVars: []string{"NTFY_VISITOR_REQUEST_LIMIT_REPLENISH"}, Value: server.DefaultVisitorRequestLimitReplenish, Usage: "interval at which burst limit is replenished (one per x)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "visitor-request-limit-exempt-hosts", Aliases: []string{"visitor_request_limit_exempt_hosts"}, EnvVars: []string{"NTFY_VISITOR_REQUEST_LIMIT_EXEMPT_HOSTS"}, Value: "", Usage: "hostnames and/or IP addresses of hosts that will be exempt from the visitor request limit"}),
	altsrc.NewIntFlag(&cli.IntFlag{Name: "visitor-message-daily-limit", Aliases: []string{"visitor_message_daily_limit"}, EnvVars: []string{"NTFY_VISITOR_MESSAGE_DAILY_LIMIT"}, Value: server.DefaultVisitorMessageDailyLimit, Usage: "max messages per visitor per day, derived from request limit if unset"}),
	altsrc.NewIntFlag(&cli.IntFlag{Name: "visitor-email-limit-burst", Aliases: []string{"visitor_email_limit_burst"}, EnvVars: []string{"NTFY_VISITOR_EMAIL_LIMIT_BURST"}, Value: server.DefaultVisitorEmailLimitBurst, Usage: "initial limit of e-mails per visitor"}),
	altsrc.NewDurationFlag(&cli.DurationFlag{Name: "visitor-email-limit-replenish", Aliases: []string{"visitor_email_limit_replenish"}, EnvVars: []string{"NTFY_VISITOR_EMAIL_LIMIT_REPLENISH"}, Value: server.DefaultVisitorEmailLimitReplenish, Usage: "interval at which burst limit is replenished (one per x)"}),
	altsrc.NewBoolFlag(&cli.BoolFlag{Name: "visitor-subscriber-rate-limiting", Aliases: []string{"visitor_subscriber_rate_limiting"}, EnvVars: []string{"NTFY_VISITOR_SUBSCRIBER_RATE_LIMITING"}, Value: false, Usage: "enables subscriber-based rate limiting"}),
	altsrc.NewBoolFlag(&cli.BoolFlag{Name: "behind-proxy", Aliases: []string{"behind_proxy", "P"}, EnvVars: []string{"NTFY_BEHIND_PROXY"}, Value: false, Usage: "if set, use X-Forwarded-For header to determine visitor IP address (for rate limiting)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "stripe-secret-key", Aliases: []string{"stripe_secret_key"}, EnvVars: []string{"NTFY_STRIPE_SECRET_KEY"}, Value: "", Usage: "key used for the Stripe API communication, this enables payments"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "stripe-webhook-key", Aliases: []string{"stripe_webhook_key"}, EnvVars: []string{"NTFY_STRIPE_WEBHOOK_KEY"}, Value: "", Usage: "key required to validate the authenticity of incoming webhooks from Stripe"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "billing-contact", Aliases: []string{"billing_contact"}, EnvVars: []string{"NTFY_BILLING_CONTACT"}, Value: "", Usage: "e-mail or website to display in upgrade dialog (only if payments are enabled)"}),
	altsrc.NewBoolFlag(&cli.BoolFlag{Name: "enable-metrics", Aliases: []string{"enable_metrics"}, EnvVars: []string{"NTFY_ENABLE_METRICS"}, Value: false, Usage: "if set, Prometheus metrics are exposed via the /metrics endpoint"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "metrics-listen-http", Aliases: []string{"metrics_listen_http"}, EnvVars: []string{"NTFY_METRICS_LISTEN_HTTP"}, Usage: "ip:port used to expose the metrics endpoint (implicitly enables metrics)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "profile-listen-http", Aliases: []string{"profile_listen_http"}, EnvVars: []string{"NTFY_PROFILE_LISTEN_HTTP"}, Usage: "ip:port used to expose the profiling endpoints (implicitly enables profiling)"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "web-push-public-key", Aliases: []string{"web_push_public_key"}, EnvVars: []string{"NTFY_WEB_PUSH_PUBLIC_KEY"}, Usage: "public key used for web push notifications"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "web-push-private-key", Aliases: []string{"web_push_private_key"}, EnvVars: []string{"NTFY_WEB_PUSH_PRIVATE_KEY"}, Usage: "private key used for web push notifications"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "web-push-file", Aliases: []string{"web_push_file"}, EnvVars: []string{"NTFY_WEB_PUSH_FILE"}, Usage: "file used to store web push subscriptions"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "web-push-email-address", Aliases: []string{"web_push_email_address"}, EnvVars: []string{"NTFY_WEB_PUSH_EMAIL_ADDRESS"}, Usage: "e-mail address of sender, required to use browser push services"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "web-push-startup-queries", Aliases: []string{"web_push_startup_queries"}, EnvVars: []string{"NTFY_WEB_PUSH_STARTUP_QUERIES"}, Usage: "queries run when the web push database is initialized"}),
	altsrc.NewIntFlag(&cli.IntFlag{Name: "message-limit", Aliases: []string{"message_limit"}, EnvVars: []string{"NTFY_MESSAGE_LIMIT"}, Value: server.DefaultMessageLengthLimit, Usage: "size limit for the mesasge in bytes"}),
)

var cmdServe = &cli.Command{
	Name:      "serve",
	Usage:     "Run the ntfy server",
	UsageText: "ntfy serve [OPTIONS..]",
	Action:    execServe,
	Category:  categoryServer,
	Flags:     flagsServe,
	Before:    initConfigFileInputSourceFunc("config", flagsServe, initLogFunc),
	Description: `Run the ntfy server and listen for incoming requests

The command will load the configuration from /etc/ntfy/server.yml. Config options can 
be overridden using the command line options.

Examples:
  ntfy serve                      # Starts server in the foreground (on port 80)
  ntfy serve --listen-http :8080  # Starts server with alternate port`,
}

func execServe(c *cli.Context) error {
	if c.NArg() > 0 {
		return errors.New("no arguments expected, see 'ntfy serve --help' for help")
	}

	// Read all the options
	config := c.String("config")
	baseURL := c.String("base-url")
	listenHTTP := c.String("listen-http")
	listenHTTPS := c.String("listen-https")
	listenUnix := c.String("listen-unix")
	listenUnixMode := c.Int("listen-unix-mode")
	keyFile := c.String("key-file")
	certFile := c.String("cert-file")
	firebaseKeyFile := c.String("firebase-key-file")
	webPushPrivateKey := c.String("web-push-private-key")
	webPushPublicKey := c.String("web-push-public-key")
	webPushFile := c.String("web-push-file")
	webPushEmailAddress := c.String("web-push-email-address")
	webPushStartupQueries := c.String("web-push-startup-queries")
	cacheFile := c.String("cache-file")
	cacheDuration := c.Duration("cache-duration")
	cacheStartupQueries := c.String("cache-startup-queries")
	cacheBatchSize := c.Int("cache-batch-size")
	cacheBatchTimeout := c.Duration("cache-batch-timeout")
	authFile := c.String("auth-file")
	authStartupQueries := c.String("auth-startup-queries")
	authDefaultAccess := c.String("auth-default-access")
	attachmentCacheDir := c.String("attachment-cache-dir")
	attachmentTotalSizeLimitStr := c.String("attachment-total-size-limit")
	attachmentFileSizeLimitStr := c.String("attachment-file-size-limit")
	attachmentExpiryDuration := c.Duration("attachment-expiry-duration")
	keepaliveInterval := c.Duration("keepalive-interval")
	managerInterval := c.Duration("manager-interval")
	disallowedTopics := c.StringSlice("disallowed-topics")
	webRoot := c.String("web-root")
	enableSignup := c.Bool("enable-signup")
	enableLogin := c.Bool("enable-login")
	enableReservations := c.Bool("enable-reservations")
	upstreamBaseURL := c.String("upstream-base-url")
	upstreamAccessToken := c.String("upstream-access-token")
	smtpSenderAddr := c.String("smtp-sender-addr")
	smtpSenderUser := c.String("smtp-sender-user")
	smtpSenderPass := c.String("smtp-sender-pass")
	smtpSenderFrom := c.String("smtp-sender-from")
	smtpServerListen := c.String("smtp-server-listen")
	smtpServerDomain := c.String("smtp-server-domain")
	smtpServerAddrPrefix := c.String("smtp-server-addr-prefix")
	twilioAccount := c.String("twilio-account")
	twilioAuthToken := c.String("twilio-auth-token")
	twilioPhoneNumber := c.String("twilio-phone-number")
	twilioVerifyService := c.String("twilio-verify-service")
	totalTopicLimit := c.Int("global-topic-limit")
	visitorSubscriptionLimit := c.Int("visitor-subscription-limit")
	visitorSubscriberRateLimiting := c.Bool("visitor-subscriber-rate-limiting")
	visitorAttachmentTotalSizeLimitStr := c.String("visitor-attachment-total-size-limit")
	visitorAttachmentDailyBandwidthLimitStr := c.String("visitor-attachment-daily-bandwidth-limit")
	visitorRequestLimitBurst := c.Int("visitor-request-limit-burst")
	visitorRequestLimitReplenish := c.Duration("visitor-request-limit-replenish")
	visitorRequestLimitExemptHosts := util.SplitNoEmpty(c.String("visitor-request-limit-exempt-hosts"), ",")
	visitorMessageDailyLimit := c.Int("visitor-message-daily-limit")
	visitorEmailLimitBurst := c.Int("visitor-email-limit-burst")
	visitorEmailLimitReplenish := c.Duration("visitor-email-limit-replenish")
	behindProxy := c.Bool("behind-proxy")
	stripeSecretKey := c.String("stripe-secret-key")
	stripeWebhookKey := c.String("stripe-webhook-key")
	billingContact := c.String("billing-contact")
	metricsListenHTTP := c.String("metrics-listen-http")
	enableMetrics := c.Bool("enable-metrics") || metricsListenHTTP != ""
	profileListenHTTP := c.String("profile-listen-http")
	messasgeLimit := c.Int("message-limit")

	// Check values
	if firebaseKeyFile != "" && !util.FileExists(firebaseKeyFile) {
		return errors.New("if set, FCM key file must exist")
	} else if webPushPublicKey != "" && (webPushPrivateKey == "" || webPushFile == "" || webPushEmailAddress == "" || baseURL == "") {
		return errors.New("if web push is enabled, web-push-private-key, web-push-public-key, web-push-file, web-push-email-address, and base-url should be set. run 'ntfy webpush keys' to generate keys")
	} else if keepaliveInterval < 5*time.Second {
		return errors.New("keepalive interval cannot be lower than five seconds")
	} else if managerInterval < 5*time.Second {
		return errors.New("manager interval cannot be lower than five seconds")
	} else if cacheDuration > 0 && cacheDuration < managerInterval {
		return errors.New("cache duration cannot be lower than manager interval")
	} else if keyFile != "" && !util.FileExists(keyFile) {
		return errors.New("if set, key file must exist")
	} else if certFile != "" && !util.FileExists(certFile) {
		return errors.New("if set, certificate file must exist")
	} else if listenHTTPS != "" && (keyFile == "" || certFile == "") {
		return errors.New("if listen-https is set, both key-file and cert-file must be set")
	} else if smtpSenderAddr != "" && (baseURL == "" || smtpSenderFrom == "") {
		return errors.New("if smtp-sender-addr is set, base-url, and smtp-sender-from must also be set")
	} else if smtpServerListen != "" && smtpServerDomain == "" {
		return errors.New("if smtp-server-listen is set, smtp-server-domain must also be set")
	} else if attachmentCacheDir != "" && baseURL == "" {
		return errors.New("if attachment-cache-dir is set, base-url must also be set")
	} else if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return errors.New("if set, base-url must start with http:// or https://")
	} else if baseURL != "" && strings.HasSuffix(baseURL, "/") {
		return errors.New("if set, base-url must not end with a slash (/)")
	} else if upstreamBaseURL != "" && !strings.HasPrefix(upstreamBaseURL, "http://") && !strings.HasPrefix(upstreamBaseURL, "https://") {
		return errors.New("if set, upstream-base-url must start with http:// or https://")
	} else if upstreamBaseURL != "" && strings.HasSuffix(upstreamBaseURL, "/") {
		return errors.New("if set, upstream-base-url must not end with a slash (/)")
	} else if upstreamBaseURL != "" && baseURL == "" {
		return errors.New("if upstream-base-url is set, base-url must also be set")
	} else if upstreamBaseURL != "" && baseURL != "" && baseURL == upstreamBaseURL {
		return errors.New("base-url and upstream-base-url cannot be identical, you'll likely want to set upstream-base-url to https://ntfy.sh, see https://ntfy.sh/docs/config/#ios-instant-notifications")
	} else if authFile == "" && (enableSignup || enableLogin || enableReservations || stripeSecretKey != "") {
		return errors.New("cannot set enable-signup, enable-login, enable-reserve-topics, or stripe-secret-key if auth-file is not set")
	} else if enableSignup && !enableLogin {
		return errors.New("cannot set enable-signup without also setting enable-login")
	} else if stripeSecretKey != "" && (stripeWebhookKey == "" || baseURL == "") {
		return errors.New("if stripe-secret-key is set, stripe-webhook-key and base-url must also be set")
	} else if twilioAccount != "" && (twilioAuthToken == "" || twilioPhoneNumber == "" || twilioVerifyService == "" || baseURL == "" || authFile == "") {
		return errors.New("if twilio-account is set, twilio-auth-token, twilio-phone-number, twilio-verify-service, base-url, and auth-file must also be set")
	}

	// Backwards compatibility
	if webRoot == "app" {
		webRoot = "/"
	} else if webRoot == "home" {
		webRoot = "/app"
	} else if webRoot == "disable" {
		webRoot = ""
	} else if !strings.HasPrefix(webRoot, "/") {
		webRoot = "/" + webRoot
	}

	// Default auth permissions
	authDefault, err := user.ParsePermission(authDefaultAccess)
	if err != nil {
		return errors.New("if set, auth-default-access must start set to 'read-write', 'read-only', 'write-only' or 'deny-all'")
	}

	// Special case: Unset default
	if listenHTTP == "-" {
		listenHTTP = ""
	}

	// Convert sizes to bytes
	attachmentTotalSizeLimit, err := parseSize(attachmentTotalSizeLimitStr, server.DefaultAttachmentTotalSizeLimit)
	if err != nil {
		return err
	}
	attachmentFileSizeLimit, err := parseSize(attachmentFileSizeLimitStr, server.DefaultAttachmentFileSizeLimit)
	if err != nil {
		return err
	}
	visitorAttachmentTotalSizeLimit, err := parseSize(visitorAttachmentTotalSizeLimitStr, server.DefaultVisitorAttachmentTotalSizeLimit)
	if err != nil {
		return err
	}
	visitorAttachmentDailyBandwidthLimit, err := parseSize(visitorAttachmentDailyBandwidthLimitStr, server.DefaultVisitorAttachmentDailyBandwidthLimit)
	if err != nil {
		return err
	} else if visitorAttachmentDailyBandwidthLimit > math.MaxInt {
		return fmt.Errorf("config option visitor-attachment-daily-bandwidth-limit must be lower than %d", math.MaxInt)
	}

	// Resolve hosts
	visitorRequestLimitExemptIPs := make([]netip.Prefix, 0)
	for _, host := range visitorRequestLimitExemptHosts {
		ips, err := parseIPHostPrefix(host)
		if err != nil {
			log.Warn("cannot resolve host %s: %s, ignoring visitor request exemption", host, err.Error())
			continue
		}
		visitorRequestLimitExemptIPs = append(visitorRequestLimitExemptIPs, ips...)
	}

	// Stripe things
	if stripeSecretKey != "" {
		stripe.EnableTelemetry = false // Whoa!
		stripe.Key = stripeSecretKey
	}

	// Add default forbidden topics
	disallowedTopics = append(disallowedTopics, server.DefaultDisallowedTopics...)

	// Run server
	conf := server.NewConfig()
	conf.File = config
	conf.BaseURL = baseURL
	conf.ListenHTTP = listenHTTP
	conf.ListenHTTPS = listenHTTPS
	conf.ListenUnix = listenUnix
	conf.ListenUnixMode = fs.FileMode(listenUnixMode)
	conf.KeyFile = keyFile
	conf.CertFile = certFile
	conf.FirebaseKeyFile = firebaseKeyFile
	conf.CacheFile = cacheFile
	conf.CacheDuration = cacheDuration
	conf.CacheStartupQueries = cacheStartupQueries
	conf.CacheBatchSize = cacheBatchSize
	conf.CacheBatchTimeout = cacheBatchTimeout
	conf.AuthFile = authFile
	conf.AuthStartupQueries = authStartupQueries
	conf.AuthDefault = authDefault
	conf.AttachmentCacheDir = attachmentCacheDir
	conf.AttachmentTotalSizeLimit = attachmentTotalSizeLimit
	conf.AttachmentFileSizeLimit = attachmentFileSizeLimit
	conf.AttachmentExpiryDuration = attachmentExpiryDuration
	conf.KeepaliveInterval = keepaliveInterval
	conf.ManagerInterval = managerInterval
	conf.DisallowedTopics = disallowedTopics
	conf.WebRoot = webRoot
	conf.UpstreamBaseURL = upstreamBaseURL
	conf.UpstreamAccessToken = upstreamAccessToken
	conf.SMTPSenderAddr = smtpSenderAddr
	conf.SMTPSenderUser = smtpSenderUser
	conf.SMTPSenderPass = smtpSenderPass
	conf.SMTPSenderFrom = smtpSenderFrom
	conf.SMTPServerListen = smtpServerListen
	conf.SMTPServerDomain = smtpServerDomain
	conf.SMTPServerAddrPrefix = smtpServerAddrPrefix
	conf.TwilioAccount = twilioAccount
	conf.TwilioAuthToken = twilioAuthToken
	conf.TwilioPhoneNumber = twilioPhoneNumber
	conf.TwilioVerifyService = twilioVerifyService
	conf.TotalTopicLimit = totalTopicLimit
	conf.VisitorSubscriptionLimit = visitorSubscriptionLimit
	conf.VisitorAttachmentTotalSizeLimit = visitorAttachmentTotalSizeLimit
	conf.VisitorAttachmentDailyBandwidthLimit = visitorAttachmentDailyBandwidthLimit
	conf.VisitorRequestLimitBurst = visitorRequestLimitBurst
	conf.VisitorRequestLimitReplenish = visitorRequestLimitReplenish
	conf.VisitorRequestExemptIPAddrs = visitorRequestLimitExemptIPs
	conf.VisitorMessageDailyLimit = visitorMessageDailyLimit
	conf.VisitorEmailLimitBurst = visitorEmailLimitBurst
	conf.VisitorEmailLimitReplenish = visitorEmailLimitReplenish
	conf.VisitorSubscriberRateLimiting = visitorSubscriberRateLimiting
	conf.BehindProxy = behindProxy
	conf.StripeSecretKey = stripeSecretKey
	conf.StripeWebhookKey = stripeWebhookKey
	conf.BillingContact = billingContact
	conf.EnableSignup = enableSignup
	conf.EnableLogin = enableLogin
	conf.EnableReservations = enableReservations
	conf.EnableMetrics = enableMetrics
	conf.MetricsListenHTTP = metricsListenHTTP
	conf.ProfileListenHTTP = profileListenHTTP
	conf.Version = c.App.Version
	conf.WebPushPrivateKey = webPushPrivateKey
	conf.WebPushPublicKey = webPushPublicKey
	conf.WebPushFile = webPushFile
	conf.WebPushEmailAddress = webPushEmailAddress
	conf.WebPushStartupQueries = webPushStartupQueries
	conf.MessageLimit = messasgeLimit

	// Set up hot-reloading of config
	go sigHandlerConfigReload(config)

	// Run server
	s, err := server.New(conf)
	if err != nil {
		log.Fatal(err.Error())
	} else if err := s.Run(); err != nil {
		log.Fatal(err.Error())
	}
	log.Info("Exiting.")
	return nil
}

func parseSize(s string, defaultValue int64) (v int64, err error) {
	if s == "" {
		return defaultValue, nil
	}
	v, err = util.ParseSize(s)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func sigHandlerConfigReload(config string) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP)
	for range sigs {
		log.Info("Partially hot reloading configuration ...")
		inputSource, err := newYamlSourceFromFile(config, flagsServe)
		if err != nil {
			log.Warn("Hot reload failed: %s", err.Error())
			continue
		}
		if err := reloadLogLevel(inputSource); err != nil {
			log.Warn("Reloading log level failed: %s", err.Error())
		}
	}
}

func parseIPHostPrefix(host string) (prefixes []netip.Prefix, err error) {
	// Try parsing as prefix, e.g. 10.0.1.0/24
	prefix, err := netip.ParsePrefix(host)
	if err == nil {
		prefixes = append(prefixes, prefix.Masked())
		return prefixes, nil
	}
	// Not a prefix, parse as host or IP (LookupHost passes through an IP as is)
	ips, err := net.LookupHost(host)
	if err != nil {
		return nil, err
	}
	for _, ipStr := range ips {
		ip, err := netip.ParseAddr(ipStr)
		if err == nil {
			prefix, err := ip.Prefix(ip.BitLen())
			if err != nil {
				return nil, fmt.Errorf("%s successfully parsed but unable to make prefix: %s", ip.String(), err.Error())
			}
			prefixes = append(prefixes, prefix.Masked())
		}
	}
	return
}

func reloadLogLevel(inputSource altsrc.InputSourceContext) error {
	newLevelStr, err := inputSource.String("log-level")
	if err != nil {
		return fmt.Errorf("cannot load log level: %s", err.Error())
	}
	overrides, err := inputSource.StringSlice("log-level-overrides")
	if err != nil {
		return fmt.Errorf("cannot load log level overrides (1): %s", err.Error())
	}
	log.ResetLevelOverrides()
	if err := applyLogLevelOverrides(overrides); err != nil {
		return fmt.Errorf("cannot load log level overrides (2): %s", err.Error())
	}
	log.SetLevel(log.ToLevel(newLevelStr))
	if len(overrides) > 0 {
		log.Info("Log level is %v, %d override(s) in place", strings.ToUpper(newLevelStr), len(overrides))
	} else {
		log.Info("Log level is %v", strings.ToUpper(newLevelStr))
	}
	return nil
}
