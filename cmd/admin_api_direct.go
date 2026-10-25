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
	"heckel.io/ntfy/v2/client"
	"heckel.io/ntfy/v2/mail"
	"heckel.io/ntfy/v2/user"
)

const (
	directPasswordResetExpiry = time.Hour // Same as the server's passwordResetTokenExpiry
	directPasswordResetPath   = "/account/password/reset/"
)

// directAdminAPI implements adminAPI against the user database, without a server. It is the
// fallback when no admin socket answers (server not running, Docker without a shared socket,
// custom systemd unit without RuntimeDirectory, Windows). It must behave like the server's
// handlers over the admin socket, including the error codes, so the commands cannot tell the
// difference. Direct access is at least as privileged as the socket, so admins may be changed.
type directAdminAPI struct {
	manager  *user.Manager
	baseURL  string
	smtpConf *mail.Config
}

var _ adminAPI = (*directAdminAPI)(nil)

func newDirectAdminAPI(c *cli.Context) (*directAdminAPI, error) {
	manager, err := createUserManager(c)
	if err != nil {
		return nil, err
	}
	return &directAdminAPI{
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

func (d *directAdminAPI) Users() ([]*client.AdminUser, error) {
	users, err := d.manager.Users()
	if err != nil {
		return nil, err
	}
	grants, err := d.manager.AllGrants()
	if err != nil {
		return nil, err
	}
	adminUsers := make([]*client.AdminUser, len(users))
	for i, u := range users {
		adminUsers[i] = d.toAdminUser(u, grants[u.ID])
	}
	return adminUsers, nil
}

func (d *directAdminAPI) User(username string) (*client.AdminUser, error) {
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

func (d *directAdminAPI) AddUser(username, hash, tier, role string) error {
	if role == "" {
		role = string(user.RoleUser)
	} else if !user.AllowedRole(user.Role(role)) {
		return directErrorCode(client.AdminErrCodeRoleInvalid)
	}
	if _, err := d.manager.User(username); err == nil {
		return directErrorCode(client.AdminErrCodeUserExists)
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

func (d *directAdminAPI) ChangePassword(username, hash string) error {
	if _, err := d.manager.User(username); err != nil {
		return directError(err)
	}
	return directError(d.manager.ChangePassword(username, hash, true))
}

func (d *directAdminAPI) ChangeTier(username, tier string) error {
	if _, err := d.manager.User(username); err != nil {
		return directError(err)
	}
	if tier == client.AdminTierReset {
		return directError(d.manager.ResetTier(username))
	}
	if _, err := d.manager.Tier(tier); err != nil {
		return directError(err)
	}
	return directError(d.manager.ChangeTier(username, tier))
}

func (d *directAdminAPI) ChangeRole(username, role string) error {
	if !user.AllowedRole(user.Role(role)) {
		return directErrorCode(client.AdminErrCodeRoleInvalid)
	}
	if _, err := d.manager.User(username); err != nil {
		return directError(err)
	}
	return directError(d.manager.ChangeRole(username, user.Role(role)))
}

func (d *directAdminAPI) RemoveUser(username string) error {
	if username == user.Everyone {
		return &client.AdminError{HTTPCode: http.StatusBadRequest, Message: "invalid request: cannot remove the everyone user"}
	}
	if _, err := d.manager.User(username); err != nil {
		return directError(err)
	}
	return directError(d.manager.RemoveUser(username))
}

func (d *directAdminAPI) ResetPassword(username string, sendEmail bool) (*client.AdminPasswordReset, error) {
	if d.baseURL == "" {
		return nil, directErrorCode(client.AdminErrCodeBaseURLNotConfigured)
	}
	u, err := d.manager.User(username)
	if err != nil {
		return nil, directError(err)
	} else if u.Provisioned {
		return nil, directErrorCode(client.AdminErrCodeProvisionedUserChange)
	}
	// Resolve the primary email up front if we need to send -- fail before creating a token
	var email string
	if sendEmail {
		if d.smtpConf.SMTPAddr == "" {
			return nil, &client.AdminError{HTTPCode: http.StatusBadRequest, Message: "smtp-sender-addr is not configured"}
		}
		if email, err = d.manager.PrimaryEmail(u.ID); err != nil {
			return nil, err
		} else if email == "" {
			return nil, directErrorCode(client.AdminErrCodeUserNoPrimaryEmail)
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
	return &client.AdminPasswordReset{
		Link:  link,
		Email: email,
	}, nil
}

func (d *directAdminAPI) AllowAccess(username, topic, permission string) error {
	u, err := d.manager.User(username)
	if err != nil {
		return directError(err)
	} else if u.IsAdmin() {
		return directErrorCode(client.AdminErrCodeAccessAdminUser)
	}
	p, err := user.ParsePermission(permission)
	if err != nil {
		return directErrorCode(client.AdminErrCodePermissionInvalid)
	}
	return directError(d.manager.AllowAccess(username, topic, p))
}

func (d *directAdminAPI) ResetAccess(username, topic string) error {
	if username != "" {
		if _, err := d.manager.User(username); err != nil {
			return directError(err)
		}
	}
	return directError(d.manager.ResetAccess(username, topic))
}

func (d *directAdminAPI) Tokens(username string) ([]*client.AdminToken, error) {
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
	adminTokens := make([]*client.AdminToken, 0)
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

func (d *directAdminAPI) AddToken(username, label string, expires time.Time) (*client.AdminToken, error) {
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

func (d *directAdminAPI) RemoveToken(username, token string) error {
	u, err := d.manager.User(username)
	if err != nil {
		return directError(err)
	}
	return directError(d.manager.RemoveToken(u.ID, token))
}

func (d *directAdminAPI) Tiers() ([]*client.AdminTier, error) {
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
	adminTiers := make([]*client.AdminTier, len(tiers))
	for i, t := range tiers {
		adminTiers[i] = toAdminTier(t, counts[t.Code])
	}
	return adminTiers, nil
}

func (d *directAdminAPI) Tier(code string) (*client.AdminTier, error) {
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

func (d *directAdminAPI) AddTier(t *client.AdminTier) error {
	if !user.AllowedTier(t.Code) || strings.TrimSpace(t.Name) == "" {
		return directErrorCode(client.AdminErrCodeTierCodeInvalid)
	}
	if _, err := d.manager.Tier(t.Code); err == nil {
		return directErrorCode(client.AdminErrCodeTierExists)
	} else if !errors.Is(err, user.ErrTierNotFound) {
		return err
	}
	return directError(d.manager.AddTier(fromAdminTier(t)))
}

func (d *directAdminAPI) UpdateTier(t *client.AdminTier) error {
	if !user.AllowedTier(t.Code) || strings.TrimSpace(t.Name) == "" {
		return directErrorCode(client.AdminErrCodeTierCodeInvalid)
	}
	if _, err := d.manager.Tier(t.Code); err != nil {
		return directError(err)
	}
	return directError(d.manager.UpdateTier(fromAdminTier(t)))
}

func (d *directAdminAPI) RemoveTier(code string) error {
	if _, err := d.manager.Tier(code); err != nil {
		return directError(err)
	}
	counts, err := d.manager.TierUsersCount()
	if err != nil {
		return err
	} else if counts[code] > 0 {
		return directErrorCode(client.AdminErrCodeTierInUse)
	}
	return directError(d.manager.RemoveTier(code))
}

func (d *directAdminAPI) Close() error {
	return d.manager.Close()
}

func (d *directAdminAPI) toAdminUser(u *user.User, grants []user.Grant) *client.AdminUser {
	adminUser := &client.AdminUser{
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
		adminUser.Grants = append(adminUser.Grants, &client.AdminGrant{
			Topic:       g.TopicPattern,
			Permission:  g.Permission.String(),
			Provisioned: g.Provisioned,
		})
	}
	return adminUser
}

func toAdminToken(username string, t *user.Token) *client.AdminToken {
	return &client.AdminToken{
		Username:    username,
		Token:       t.Value,
		Label:       t.Label,
		Expires:     t.Expires.Unix(),
		LastAccess:  t.LastAccess.Unix(),
		LastOrigin:  t.LastOrigin.String(),
		Provisioned: t.Provisioned,
	}
}

func toAdminTier(t *user.Tier, users int64) *client.AdminTier {
	return &client.AdminTier{
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

func fromAdminTier(t *client.AdminTier) *user.Tier {
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
		return directErrorCode(client.AdminErrCodeUserNotFound)
	case errors.Is(err, user.ErrUserExists):
		return directErrorCode(client.AdminErrCodeUserExists)
	case errors.Is(err, user.ErrTierNotFound):
		return directErrorCode(client.AdminErrCodeTierNotFound)
	case errors.Is(err, user.ErrTokenNotFound):
		return directErrorCode(client.AdminErrCodeTokenNotFound)
	case errors.Is(err, user.ErrProvisionedUserChange):
		return directErrorCode(client.AdminErrCodeProvisionedUserChange)
	case errors.Is(err, user.ErrProvisionedTokenChange):
		return directErrorCode(client.AdminErrCodeProvisionedTokenChange)
	case errors.Is(err, user.ErrTooManyReservations):
		return directErrorCode(client.AdminErrCodeTierReservationsLimit)
	case errors.Is(err, user.ErrPasswordHashInvalid), errors.Is(err, user.ErrPasswordHashWeak):
		return directErrorCode(client.AdminErrCodePasswordHashInvalid)
	}
	return err
}

// directErrorCode returns an admin API error with the given code. The message is never shown:
// adminError replaces it with the CLI's own wording for every code used here.
func directErrorCode(code int) error {
	return &client.AdminError{
		HTTPCode: http.StatusBadRequest,
		Code:     code,
		Message:  "invalid request",
	}
}
