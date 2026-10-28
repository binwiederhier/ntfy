package visitor

import (
	"time"

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
		RequestLimitReplenish:    fasterRefill(conf.RequestLimitReplenish, dailyLimitToRefill(tier.MessageLimit*messageToRequestLimitReplenishFactor)),
		MessageLimit:             tier.MessageLimit,
		MessageExpiryDuration:    tier.MessageExpiryDuration,
		EmailLimit:               tier.EmailLimit,
		EmailLimitBurst:          util.MinMax(int(float64(tier.EmailLimit)*emailLimitBurstRate), conf.EmailLimitBurst, emailLimitBurstMax),
		EmailLimitReplenish:      dailyLimitToRefill(tier.EmailLimit),
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
		RequestLimitReplenish:    conf.RequestLimitReplenish,
		MessageLimit:             messagesLimit,
		MessageExpiryDuration:    conf.MessageExpiryDuration,
		EmailLimit:               emailLimit,
		EmailLimitBurst:          conf.EmailLimitBurst,
		EmailLimitReplenish:      conf.EmailLimitReplenish,
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

// dailyLimitToRefill returns the time to refill one token so that a bucket refills limit tokens
// per day; 0 (never refills) for a limit of 0
func dailyLimitToRefill(limit int64) time.Duration {
	if limit <= 0 {
		return 0
	}
	return oneDay / time.Duration(limit)
}

// fasterRefill returns the faster of two refill intervals (the shorter one); 0 means no refill
// and never wins
func fasterRefill(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
	} else if b <= 0 {
		return a
	}
	return min(a, b)
}
