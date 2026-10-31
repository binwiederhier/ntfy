//go:build !noserver

package cmd

import (
	"errors"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/v2/mail"
	"heckel.io/ntfy/v2/user"
)

const (
	directPasswordResetExpiry = time.Hour // Same as the server's passwordResetTokenExpiry
	directPasswordResetPath   = "/account/password/reset/"
)

// dbBackend implements backend against the user database, without a server. It is the
// fallback when no admin socket answers (server not running, Docker without a shared socket,
// custom systemd unit without RuntimeDirectory, Windows). It must behave like the server's
// handlers over the admin socket, including the error codes, so the commands cannot tell the
// difference. Direct access is at least as privileged as the socket, so admins may be changed.
type dbBackend struct {
	manager  *user.Manager
	baseURL  string
	smtpConf *mail.Config
}

var _ backend = (*dbBackend)(nil)

func newDBBackend(c *cli.Context) (*dbBackend, error) {
	manager, err := createUserManager(c)
	if err != nil {
		return nil, err
	}
	return &dbBackend{
		manager: manager,
		baseURL: strings.TrimSuffix(c.String("base-url"), "/"),
		smtpConf: &mail.Config{
			SMTPAddr: c.String("smtp-sender-addr"),
			SMTPUser: c.String("smtp-sender-user"),
			SMTPPass: c.String("smtp-sender-pass"),
			From:     c.String("smtp-sender-from"),
		},
	}, nil
}

func (d *dbBackend) Users() ([]*apiUser, error) {
	users, err := d.manager.Users()
	if err != nil {
		return nil, err
	}
	grants, err := d.manager.AllGrants()
	if err != nil {
		return nil, err
	}
	adminUsers := make([]*apiUser, len(users))
	for i, u := range users {
		adminUsers[i] = d.toAdminUser(u, grants[u.ID])
	}
	return adminUsers, nil
}

func (d *dbBackend) User(username string) (*apiUser, error) {
	u, err := d.manager.User(username)
	if err != nil {
		return nil, directError(err)
	}
	grants, err := d.manager.Grants(username)
	if err != nil {
		return nil, err
	}
	return d.toAdminUser(u, grants), nil
}

func (d *dbBackend) AddUser(username, hash, tier, role string) error {
	if role == "" {
		role = string(user.RoleUser)
	} else if !user.AllowedRole(user.Role(role)) {
		return directErrorCode(apiErrCodeRoleInvalid)
	}
	if _, err := d.manager.User(username); err == nil {
		return directErrorCode(apiErrCodeUserExists)
	} else if !errors.Is(err, user.ErrUserNotFound) {
		return err
	}
	if tier != "" {
		if _, err := d.manager.Tier(tier); err != nil {
			return directError(err)
		}
	}
	if err := d.manager.AddUser(username, hash, user.Role(role), true); err != nil {
		return directError(err)
	}
	if tier != "" {
		return directError(d.manager.ChangeTier(username, tier))
	}
	return nil
}

func (d *dbBackend) ChangePassword(username, hash string) error {
	if _, err := d.manager.User(username); err != nil {
		return directError(err)
	}
	return directError(d.manager.ChangePassword(username, hash, true))
}

func (d *dbBackend) ChangeTier(username, tier string) error {
	if _, err := d.manager.User(username); err != nil {
		return directError(err)
	}
	if tier == tierReset {
		return directError(d.manager.ResetTier(username))
	}
	if _, err := d.manager.Tier(tier); err != nil {
		return directError(err)
	}
	return directError(d.manager.ChangeTier(username, tier))
}

func (d *dbBackend) ChangeRole(username, role string) error {
	if !user.AllowedRole(user.Role(role)) {
		return directErrorCode(apiErrCodeRoleInvalid)
	}
	if _, err := d.manager.User(username); err != nil {
		return directError(err)
	}
	return directError(d.manager.ChangeRole(username, user.Role(role)))
}

func (d *dbBackend) RemoveUser(username string) error {
	if username == user.Everyone {
		return &apiError{HTTPCode: http.StatusBadRequest, Message: "invalid request: cannot remove the everyone user"}
	}
	if _, err := d.manager.User(username); err != nil {
		return directError(err)
	}
	return directError(d.manager.RemoveUser(username))
}

func (d *dbBackend) ResetPassword(username string, sendEmail bool) (*apiPasswordReset, error) {
	if d.baseURL == "" {
		return nil, directErrorCode(apiErrCodeBaseURLNotConfigured)
	}
	u, err := d.manager.User(username)
	if err != nil {
		return nil, directError(err)
	} else if u.Provisioned {
		return nil, directErrorCode(apiErrCodeProvisionedUserChange)
	}
	// Resolve the primary email up front if we need to send -- fail before creating a token
	var email string
	if sendEmail {
		if d.smtpConf.SMTPAddr == "" {
			return nil, &apiError{HTTPCode: http.StatusBadRequest, Message: "smtp-sender-addr is not configured"}
		}
		if email, err = d.manager.PrimaryEmail(u.ID); err != nil {
			return nil, err
		} else if email == "" {
			return nil, directErrorCode(apiErrCodeUserNoPrimaryEmail)
		}
	}
	// The reset token is bound to the user, not an email -- so this works even with no SMTP
	token, err := d.manager.AddMagicLink(user.MagicLinkKindPasswordReset, u.ID, "", directPasswordResetExpiry)
	if err != nil {
		return nil, err
	}
	link := d.baseURL + directPasswordResetPath + token
	if sendEmail {
		if err := mail.NewSender(d.smtpConf).SendPasswordReset(email, link); err != nil {
			return nil, err
		}
	}
	return &apiPasswordReset{
		Link:  link,
		Email: email,
	}, nil
}

func (d *dbBackend) AllowAccess(username, topic, permission string) error {
	u, err := d.manager.User(username)
	if err != nil {
		return directError(err)
	} else if u.IsAdmin() {
		return directErrorCode(apiErrCodeAccessAdminUser)
	}
	p, err := user.ParsePermission(permission)
	if err != nil {
		return directErrorCode(apiErrCodePermissionInvalid)
	}
	return directError(d.manager.AllowAccess(username, topic, p))
}

func (d *dbBackend) ResetAccess(username, topic string) error {
	if username != "" {
		if _, err := d.manager.User(username); err != nil {
			return directError(err)
		}
	}
	return directError(d.manager.ResetAccess(username, topic))
}

func (d *dbBackend) Tokens(username string) ([]*apiToken, error) {
	var users []*user.User
	if username != "" {
		u, err := d.manager.User(username)
		if err != nil {
			return nil, directError(err)
		}
		users = []*user.User{u}
	} else {
		var err error
		if users, err = d.manager.Users(); err != nil {
			return nil, err
		}
	}
	adminTokens := make([]*apiToken, 0)
	for _, u := range users {
		tokens, err := d.manager.Tokens(u.ID)
		if err != nil {
			return nil, err
		}
		for _, t := range tokens {
			adminTokens = append(adminTokens, toAdminToken(u.Name, t))
		}
	}
	return adminTokens, nil
}

func (d *dbBackend) AddToken(username, label string, expires time.Time) (*apiToken, error) {
	u, err := d.manager.User(username)
	if err != nil {
		return nil, directError(err)
	}
	if expires.IsZero() {
		expires = time.Unix(0, 0)
	}
	token, err := d.manager.CreateToken(u.ID, label, expires, netip.IPv4Unspecified(), false)
	if err != nil {
		return nil, directError(err)
	}
	return toAdminToken(u.Name, token), nil
}

func (d *dbBackend) RemoveToken(username, token string) error {
	u, err := d.manager.User(username)
	if err != nil {
		return directError(err)
	}
	return directError(d.manager.RemoveToken(u.ID, token))
}

func (d *dbBackend) Tiers() ([]*apiTier, error) {
	tiers, err := d.manager.Tiers()
	if err != nil {
		return nil, err
	}
	counts, err := d.manager.TierUsersCount()
	if err != nil {
		return nil, err
	}
	slices.SortFunc(tiers, func(a, b *user.Tier) int {
		return strings.Compare(a.Code, b.Code)
	})
	adminTiers := make([]*apiTier, len(tiers))
	for i, t := range tiers {
		adminTiers[i] = toAdminTier(t, counts[t.Code])
	}
	return adminTiers, nil
}

func (d *dbBackend) Tier(code string) (*apiTier, error) {
	t, err := d.manager.Tier(code)
	if err != nil {
		return nil, directError(err)
	}
	counts, err := d.manager.TierUsersCount()
	if err != nil {
		return nil, err
	}
	return toAdminTier(t, counts[t.Code]), nil
}

func (d *dbBackend) AddTier(t *apiTier) error {
	if !user.AllowedTier(t.Code) || strings.TrimSpace(t.Name) == "" {
		return directErrorCode(apiErrCodeTierCodeInvalid)
	}
	if _, err := d.manager.Tier(t.Code); err == nil {
		return directErrorCode(apiErrCodeTierExists)
	} else if !errors.Is(err, user.ErrTierNotFound) {
		return err
	}
	return directError(d.manager.AddTier(fromAdminTier(t)))
}

func (d *dbBackend) UpdateTier(t *apiTier) error {
	if !user.AllowedTier(t.Code) || strings.TrimSpace(t.Name) == "" {
		return directErrorCode(apiErrCodeTierCodeInvalid)
	}
	if _, err := d.manager.Tier(t.Code); err != nil {
		return directError(err)
	}
	return directError(d.manager.UpdateTier(fromAdminTier(t)))
}

func (d *dbBackend) RemoveTier(code string) error {
	if _, err := d.manager.Tier(code); err != nil {
		return directError(err)
	}
	counts, err := d.manager.TierUsersCount()
	if err != nil {
		return err
	} else if counts[code] > 0 {
		return directErrorCode(apiErrCodeTierInUse)
	}
	return directError(d.manager.RemoveTier(code))
}

func (d *dbBackend) Close() error {
	return d.manager.Close()
}

func (d *dbBackend) toAdminUser(u *user.User, grants []user.Grant) *apiUser {
	adminUser := &apiUser{
		Username:    u.Name,
		Role:        string(u.Role),
		Provisioned: u.Provisioned,
	}
	if u.Tier != nil {
		adminUser.Tier = u.Tier.Code
		adminUser.TierName = u.Tier.Name
	}
	if u.Name == user.Everyone {
		adminUser.DefaultAccess = d.manager.DefaultAccess().String()
	}
	for _, g := range grants {
		adminUser.Grants = append(adminUser.Grants, &apiGrant{
			Topic:       g.TopicPattern,
			Permission:  g.Permission.String(),
			Provisioned: g.Provisioned,
		})
	}
	return adminUser
}

func toAdminToken(username string, t *user.Token) *apiToken {
	token := &apiToken{
		Username:    username,
		Token:       t.Value,
		Label:       t.Label,
		Expires:     t.Expires.Unix(),
		Provisioned: t.Provisioned,
	}
	if t.LastOrigin.IsValid() && !t.LastOrigin.IsUnspecified() { // Never used, as the server reports it (see newAPIUserTokenResponse)
		token.LastAccess = t.LastAccess.Unix()
		token.LastOrigin = t.LastOrigin.String()
	}
	return token
}

func toAdminTier(t *user.Tier, users int64) *apiTier {
	return &apiTier{
		ID:                       t.ID,
		Code:                     t.Code,
		Name:                     t.Name,
		MessagesLimit:            t.MessageLimit,
		MessagesExpiryDuration:   int64(t.MessageExpiryDuration.Seconds()),
		EmailsLimit:              t.EmailLimit,
		CallsLimit:               t.CallLimit,
		ReservationsLimit:        t.ReservationLimit,
		AttachmentFileSizeLimit:  t.AttachmentFileSizeLimit,
		AttachmentTotalSizeLimit: t.AttachmentTotalSizeLimit,
		AttachmentExpiryDuration: int64(t.AttachmentExpiryDuration.Seconds()),
		AttachmentBandwidthLimit: t.AttachmentBandwidthLimit,
		StripeMonthlyPriceID:     t.StripeMonthlyPriceID,
		StripeYearlyPriceID:      t.StripeYearlyPriceID,
		Users:                    users,
	}
}

func fromAdminTier(t *apiTier) *user.Tier {
	return &user.Tier{
		Code:                     t.Code,
		Name:                     strings.TrimSpace(t.Name),
		MessageLimit:             t.MessagesLimit,
		MessageExpiryDuration:    time.Duration(t.MessagesExpiryDuration) * time.Second,
		EmailLimit:               t.EmailsLimit,
		CallLimit:                t.CallsLimit,
		ReservationLimit:         t.ReservationsLimit,
		AttachmentFileSizeLimit:  t.AttachmentFileSizeLimit,
		AttachmentTotalSizeLimit: t.AttachmentTotalSizeLimit,
		AttachmentExpiryDuration: time.Duration(t.AttachmentExpiryDuration) * time.Second,
		AttachmentBandwidthLimit: t.AttachmentBandwidthLimit,
		StripeMonthlyPriceID:     t.StripeMonthlyPriceID,
		StripeYearlyPriceID:      t.StripeYearlyPriceID,
	}
}

// directError maps user manager errors to the admin API errors the server returns for them
// (see mapUserErrorToHTTP in server/server_admin.go), so the commands see the same codes
func directError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, user.ErrUserNotFound):
		return directErrorCode(apiErrCodeUserNotFound)
	case errors.Is(err, user.ErrUserExists):
		return directErrorCode(apiErrCodeUserExists)
	case errors.Is(err, user.ErrTierNotFound):
		return directErrorCode(apiErrCodeTierNotFound)
	case errors.Is(err, user.ErrTokenNotFound):
		return directErrorCode(apiErrCodeTokenNotFound)
	case errors.Is(err, user.ErrProvisionedUserChange):
		return directErrorCode(apiErrCodeProvisionedUserChange)
	case errors.Is(err, user.ErrProvisionedTokenChange):
		return directErrorCode(apiErrCodeProvisionedTokenChange)
	case errors.Is(err, user.ErrTooManyReservations):
		return directErrorCode(apiErrCodeTierReservationsLimit)
	case errors.Is(err, user.ErrPasswordHashInvalid), errors.Is(err, user.ErrPasswordHashWeak):
		return directErrorCode(apiErrCodePasswordHashInvalid)
	}
	return err
}

// directErrorCode returns an admin API error with the given code. The message is never shown:
// adminError replaces it with the CLI's own wording for every code used here.
func directErrorCode(code int) error {
	return &apiError{
		HTTPCode: http.StatusBadRequest,
		Code:     code,
		Message:  "invalid request",
	}
}
