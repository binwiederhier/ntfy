package visitor

import (
	"net/netip"

	"heckel.io/ntfy/v2/user"
)

// Key returns the identifier a visitor's limits are keyed by: "user:<id>" for a user with a tier,
// or "ip:<prefix>" otherwise, with the IP masked to the configured prefix bits (see Mask). A user
// without a tier shares the IP key with everyone behind the same prefix.
func Key(ip netip.Addr, u *user.User, conf *Config) string {
	if u != nil && u.Tier != nil {
		return "user:" + u.ID
	}
	return "ip:" + Mask(ip, conf).String()
}

// Mask returns the IP masked to the configured prefix bits: what the visitors behind one prefix
// share, and what is kept of a subscriber's IP (see package topic). Masking twice is harmless.
func Mask(ip netip.Addr, conf *Config) netip.Addr {
	if ip.Is4() {
		return netip.PrefixFrom(ip, conf.PrefixBitsIPv4).Masked().Addr()
	} else if ip.Is6() {
		return netip.PrefixFrom(ip, conf.PrefixBitsIPv6).Masked().Addr()
	}
	return ip
}
