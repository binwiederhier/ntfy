package visitor

import (
	"time"

	"golang.org/x/time/rate"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

// LimitsFor resolves the limits for a visitor: from the tier if there is one, or from the config
// defaults (tier nil)
func LimitsFor(conf *Config, tier *user.Tier) *Limits {
	if tier != nil {
		return tierBasedLimits(conf, tier)
	}
	return configBasedLimits(conf)
}

func tierBasedLimits(conf *Config, tier *user.Tier) *Limits {
	return &Limits{
		Basis:                    BasisTier,
		RequestLimitBurst:        util.MinMax(int(float64(tier.MessageLimit)*messageToRequestLimitBurstRate), conf.RequestLimitBurst, messageToRequestLimitBurstMax),
		RequestLimitReplenish:    util.Max(rate.Every(conf.RequestLimitReplenish), dailyLimitToRate(tier.MessageLimit*messageToRequestLimitReplenishFactor)),
		MessageLimit:             tier.MessageLimit,
		MessageExpiryDuration:    tier.MessageExpiryDuration,
		EmailLimit:               tier.EmailLimit,
		EmailLimitBurst:          util.MinMax(int(float64(tier.EmailLimit)*emailLimitBurstRate), conf.EmailLimitBurst, emailLimitBurstMax),
		EmailLimitReplenish:      dailyLimitToRate(tier.EmailLimit),
		CallLimit:                tier.CallLimit,
		ReservationsLimit:        tier.ReservationLimit,
		AttachmentTotalSizeLimit: tier.AttachmentTotalSizeLimit,
		AttachmentFileSizeLimit:  tier.AttachmentFileSizeLimit,
		AttachmentExpiryDuration: tier.AttachmentExpiryDuration,
		AttachmentBandwidthLimit: tier.AttachmentBandwidthLimit,
	}
}

func configBasedLimits(conf *Config) *Limits {
	messagesLimit := replenishDurationToDailyLimit(conf.RequestLimitReplenish) // Approximation!
	if conf.MessageDailyLimit > 0 {
		messagesLimit = int64(conf.MessageDailyLimit)
	}
	var emailLimit int64
	if conf.EmailLimitBurst > 0 {
		emailLimit = replenishDurationToDailyLimit(conf.EmailLimitReplenish) // Approximation!
	}
	return &Limits{
		Basis:                    BasisIP,
		RequestLimitBurst:        conf.RequestLimitBurst,
		RequestLimitReplenish:    rate.Every(conf.RequestLimitReplenish),
		MessageLimit:             messagesLimit,
		MessageExpiryDuration:    conf.MessageExpiryDuration,
		EmailLimit:               emailLimit,
		EmailLimitBurst:          conf.EmailLimitBurst,
		EmailLimitReplenish:      rate.Every(conf.EmailLimitReplenish),
		CallLimit:                DefaultCallsLimit,
		ReservationsLimit:        DefaultReservationsLimit,
		AttachmentTotalSizeLimit: conf.AttachmentTotalSizeLimit,
		AttachmentFileSizeLimit:  conf.AttachmentFileSizeLimit,
		AttachmentExpiryDuration: conf.AttachmentExpiryDuration,
		AttachmentBandwidthLimit: conf.AttachmentDailyBandwidthLimit,
	}
}

func replenishDurationToDailyLimit(duration time.Duration) int64 {
	return int64(oneDay / duration)
}

func dailyLimitToRate(limit int64) rate.Limit {
	return rate.Limit(limit) * rate.Every(oneDay)
}
