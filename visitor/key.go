package visitor

import (
	"net/netip"

	"heckel.io/ntfy/v2/user"
)

// Prefixes of the two kinds of keys
const (
	KeyPrefixIP   = "ip:"
	KeyPrefixUser = "user:"
)

// Key returns the identifier a visitor's limits are keyed by: "user:<id>" for a user with a tier,
// or "ip:<prefix>" otherwise, with the IP masked to the configured prefix bits. A user without a
// tier shares the IP key with everyone behind the same prefix.
func Key(ip netip.Addr, u *user.User, conf *Config) string {
	if u != nil && u.Tier != nil {
		return KeyPrefixUser + u.ID
	}
	if ip.Is4() {
		ip = netip.PrefixFrom(ip, conf.PrefixBitsIPv4).Masked().Addr()
	} else if ip.Is6() {
		ip = netip.PrefixFrom(ip, conf.PrefixBitsIPv6).Masked().Addr()
	}
	return KeyPrefixIP + ip.String()
}
