package user

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/util"
)

const (
	// User queries
	sqliteSelectUsersQuery = `
		SELECT u.id, u.user, u.pass, u.role, u.prefs, u.sync_topic, u.provisioned, u.stats_messages, u.stats_emails, u.stats_calls, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.calls_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM user u
		LEFT JOIN tier t on t.id = u.tier_id
		ORDER BY
			CASE u.role
				WHEN 'admin' THEN 1
				WHEN 'anonymous' THEN 3
				ELSE 2
			END, u.user
	`
	sqliteSelectUserByIDQuery = `
		SELECT u.id, u.user, u.pass, u.role, u.prefs, u.sync_topic, u.provisioned, u.stats_messages, u.stats_emails, u.stats_calls, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.calls_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM user u
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE u.id = ?
	`
	sqliteSelectUserByNameQuery = `
		SELECT u.id, u.user, u.pass, u.role, u.prefs, u.sync_topic, u.provisioned, u.stats_messages, u.stats_emails, u.stats_calls, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.calls_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM user u
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE user = ?
	`
	sqliteSelectUserByTokenQuery = `
		SELECT u.id, u.user, u.pass, u.role, u.prefs, u.sync_topic, u.provisioned, u.stats_messages, u.stats_emails, u.stats_calls, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.calls_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM user u
		JOIN user_token tk on u.id = tk.user_id
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE tk.token = ? AND (tk.expires = 0 OR tk.expires >= ?)
	`
	sqliteSelectUserByStripeCustomerIDQuery = `
		SELECT u.id, u.user, u.pass, u.role, u.prefs, u.sync_topic, u.provisioned, u.stats_messages, u.stats_emails, u.stats_calls, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.calls_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM user u
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE u.stripe_customer_id = ?
	`
	sqliteSelectUsernamesQuery = `
		SELECT user
		FROM user
		ORDER BY
			CASE role
				WHEN 'admin' THEN 1
				WHEN 'anonymous' THEN 3
				ELSE 2
			END, user
	`
	sqliteSelectUserCountQuery          = `SELECT COUNT(*) FROM user`
	sqliteSelectUserIDFromUsernameQuery = `SELECT id FROM user WHERE user = ?`
	sqliteInsertUserQuery               = `INSERT INTO user (id, user, pass, role, sync_topic, provisioned, created) VALUES (?, ?, ?, ?, ?, ?, ?)`
	sqliteUpdateUserPassQuery           = `UPDATE user SET pass = ? WHERE user = ?`
	sqliteUpdateUserRoleQuery           = `UPDATE user SET role = ? WHERE user = ?`
	sqliteUpdateUserProvisionedQuery    = `UPDATE user SET provisioned = ? WHERE user = ?`
	sqliteUpdateUserPrefsQuery          = `UPDATE user SET prefs = ? WHERE id = ?`
	sqliteUpdateUserStatsQuery          = `UPDATE user SET stats_messages = ?, stats_emails = ?, stats_calls = ? WHERE id = ?`
	sqliteUpdateUserStatsResetAllQuery  = `UPDATE user SET stats_messages = 0, stats_emails = 0, stats_calls = 0`
	sqliteUpdateUserTierQuery           = `UPDATE user SET tier_id = (SELECT id FROM tier WHERE code = ?) WHERE user = ?`
	sqliteUpdateUserDeletedQuery        = `UPDATE user SET deleted = ? WHERE id = ?`
	sqliteDeleteUserQuery               = `DELETE FROM user WHERE user = ?`
	sqliteDeleteUserTierQuery           = `UPDATE user SET tier_id = null WHERE user = ?`
	sqliteDeleteUsersMarkedQuery        = `DELETE FROM user WHERE deleted < ?`
	sqliteDeleteUsersProvisionedQuery   = `DELETE FROM user WHERE provisioned = 1`

	// Access queries
	sqliteSelectTopicPermsQuery = `
		SELECT read, write
		FROM user_access a
		JOIN user u ON u.id = a.user_id
		WHERE (u.user = ? OR u.user = ?) AND ? LIKE a.topic ESCAPE '\'
		ORDER BY u.user DESC, LENGTH(a.topic) DESC, a.write DESC
	`
	sqliteSelectUserAllAccessQuery = `
		SELECT user_id, topic, read, write, provisioned
		FROM user_access
		ORDER BY LENGTH(topic) DESC, write DESC, read DESC, topic
	`
	sqliteSelectUserAccessQuery = `
		SELECT topic, read, write, provisioned
		FROM user_access
		WHERE user_id = (SELECT id FROM user WHERE user = ?)
		ORDER BY LENGTH(topic) DESC, write DESC, read DESC, topic
	`
	sqliteSelectUserReservationsQuery = `
		SELECT a_user.topic, a_user.read, a_user.write, a_everyone.read AS everyone_read, a_everyone.write AS everyone_write
		FROM user_access a_user
		LEFT JOIN  user_access a_everyone ON a_user.topic = a_everyone.topic AND a_everyone.user_id = (SELECT id FROM user WHERE user = ?)
		WHERE a_user.user_id = a_user.owner_user_id
		  AND a_user.owner_user_id = (SELECT id FROM user WHERE user = ?)
		ORDER BY a_user.topic
	`
	sqliteSelectUserReservationsCountQuery = `
		SELECT COUNT(*)
		FROM user_access
		WHERE user_id = owner_user_id
		  AND owner_user_id = (SELECT id FROM user WHERE user = ?)
	`
	sqliteSelectUserReservationsOwnerQuery = `
		SELECT owner_user_id
		FROM user_access
		WHERE topic = ?
		  AND user_id = owner_user_id
	`
	sqliteSelectUserHasReservationQuery = `
		SELECT COUNT(*)
		FROM user_access
		WHERE user_id = owner_user_id
		  AND owner_user_id = (SELECT id FROM user WHERE user = ?)
		  AND topic = ?
	`
	sqliteSelectOtherAccessCountQuery = `
		SELECT COUNT(*)
		FROM user_access
		WHERE (topic = ? OR ? LIKE topic ESCAPE '\')
		  AND (owner_user_id IS NULL OR owner_user_id != (SELECT id FROM user WHERE user = ?))
	`
	sqliteUpsertUserAccessQuery = `
		INSERT INTO user_access (user_id, topic, read, write, owner_user_id, provisioned)
		VALUES ((SELECT id FROM user WHERE user = ?), ?, ?, ?, (SELECT IIF(?='',NULL,(SELECT id FROM user WHERE user=?))), ?)
		ON CONFLICT (user_id, topic)
		DO UPDATE SET read=excluded.read, write=excluded.write, owner_user_id=excluded.owner_user_id, provisioned=excluded.provisioned
	`
	sqliteDeleteUserAccessQuery = `
		DELETE FROM user_access
		WHERE user_id = (SELECT id FROM user WHERE user = ?)
		   OR owner_user_id = (SELECT id FROM user WHERE user = ?)
	`
	sqliteDeleteUserAccessProvisionedQuery = `DELETE FROM user_access WHERE provisioned = 1`
	sqliteDeleteTopicAccessQuery           = `
		DELETE FROM user_access
	   	WHERE (user_id = (SELECT id FROM user WHERE user = ?) OR owner_user_id = (SELECT id FROM user WHERE user = ?))
	   	  AND topic = ?
  	`
	sqliteDeleteAllAccessQuery = `DELETE FROM user_access`

	// Token queries
	sqliteSelectTokenQuery                = `SELECT token, label, last_access, last_origin, expires, provisioned FROM user_token WHERE user_id = ? AND token = ?`
	sqliteSelectTokensQuery               = `SELECT token, label, last_access, last_origin, expires, provisioned FROM user_token WHERE user_id = ?`
	sqliteSelectTokenCountQuery           = `SELECT COUNT(*) FROM user_token WHERE user_id = ?`
	sqliteSelectAllProvisionedTokensQuery = `SELECT token, label, last_access, last_origin, expires, provisioned FROM user_token WHERE provisioned = 1`
	sqliteUpsertTokenQuery                = `
		INSERT INTO user_token (user_id, token, label, last_access, last_origin, expires, provisioned)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, token)
		DO UPDATE SET label = excluded.label, expires = excluded.expires, provisioned = excluded.provisioned
	`
	sqliteUpdateTokenQuery                = `UPDATE user_token SET label = ?, expires = ? WHERE user_id = ? AND token = ?`
	sqliteUpdateTokenLastAccessQuery      = `UPDATE user_token SET last_access = ?, last_origin = ? WHERE token = ?`
	sqliteDeleteTokenQuery                = `DELETE FROM user_token WHERE user_id = ? AND token = ?`
	sqliteDeleteProvisionedTokenQuery     = `DELETE FROM user_token WHERE token = ?`
	sqliteDeleteAllProvisionedTokensQuery = `DELETE FROM user_token WHERE provisioned = 1`
	sqliteDeleteAllTokenQuery             = `DELETE FROM user_token WHERE user_id = ?`
	sqliteDeleteExpiredTokensQuery        = `DELETE FROM user_token WHERE expires > 0 AND expires < ?`
	sqliteDeleteExcessTokensQuery         = `
		DELETE FROM user_token
		WHERE user_id = ?
		  AND (user_id, token) NOT IN (
			SELECT user_id, token
			FROM user_token
			WHERE user_id = ?
			ORDER BY expires DESC
			LIMIT ?
		)
	`

	// Tier queries
	sqliteInsertTierQuery = `
		INSERT INTO tier (id, code, name, messages_limit, messages_expiry_duration, emails_limit, calls_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	sqliteUpdateTierQuery = `
		UPDATE tier
		SET name = ?, messages_limit = ?, messages_expiry_duration = ?, emails_limit = ?, calls_limit = ?, reservations_limit = ?, attachment_file_size_limit = ?, attachment_total_size_limit = ?, attachment_expiry_duration = ?, attachment_bandwidth_limit = ?, stripe_monthly_price_id = ?, stripe_yearly_price_id = ?
		WHERE code = ?
	`
	sqliteSelectTiersQuery = `
		SELECT id, code, name, messages_limit, messages_expiry_duration, emails_limit, calls_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id
		FROM tier
	`
	sqliteSelectTierByCodeQuery = `
		SELECT id, code, name, messages_limit, messages_expiry_duration, emails_limit, calls_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id
		FROM tier
		WHERE code = ?
	`
	sqliteSelectTierByPriceIDQuery = `
		SELECT id, code, name, messages_limit, messages_expiry_duration, emails_limit, calls_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id
		FROM tier
		WHERE (stripe_monthly_price_id = ? OR stripe_yearly_price_id = ?)
	`
	sqliteDeleteTierQuery = `DELETE FROM tier WHERE code = ?`

	// Phone queries
	sqliteSelectPhoneNumbersQuery = `SELECT phone_number FROM user_phone WHERE user_id = ?`
	sqliteInsertPhoneNumberQuery  = `INSERT INTO user_phone (user_id, phone_number) VALUES (?, ?)`
	sqliteDeletePhoneNumberQuery  = `DELETE FROM user_phone WHERE user_id = ? AND phone_number = ?`

	// Email queries
	sqliteSelectEmailsQuery = `SELECT email FROM user_email WHERE user_id = ? ORDER BY email`
	sqliteInsertEmailQuery  = `INSERT INTO user_email (user_id, email) VALUES (?, ?)`
	sqliteDeleteEmailQuery  = `DELETE FROM user_email WHERE user_id = ? AND email = ?`

	// Billing queries
	sqliteUpdateBillingQuery = `
		UPDATE user
		SET stripe_customer_id = ?, stripe_subscription_id = ?, stripe_subscription_status = ?, stripe_subscription_interval = ?, stripe_subscription_paid_until = ?, stripe_subscription_cancel_at = ?
		WHERE user = ?
	`
)

var sqliteQueries = queries{
	selectUserByID:               sqliteSelectUserByIDQuery,
	selectUserByName:             sqliteSelectUserByNameQuery,
	selectUserByToken:            sqliteSelectUserByTokenQuery,
	selectUserByStripeCustomerID: sqliteSelectUserByStripeCustomerIDQuery,
	selectUsernames:              sqliteSelectUsernamesQuery,
	selectUsers:                  sqliteSelectUsersQuery,
	selectUserCount:              sqliteSelectUserCountQuery,
	selectUserIDFromUsername:     sqliteSelectUserIDFromUsernameQuery,
	insertUser:                   sqliteInsertUserQuery,
	updateUserPass:               sqliteUpdateUserPassQuery,
	updateUserRole:               sqliteUpdateUserRoleQuery,
	updateUserProvisioned:        sqliteUpdateUserProvisionedQuery,
	updateUserPrefs:              sqliteUpdateUserPrefsQuery,
	updateUserStats:              sqliteUpdateUserStatsQuery,
	updateUserStatsResetAll:      sqliteUpdateUserStatsResetAllQuery,
	updateUserTier:               sqliteUpdateUserTierQuery,
	updateUserDeleted:            sqliteUpdateUserDeletedQuery,
	deleteUser:                   sqliteDeleteUserQuery,
	deleteUserTier:               sqliteDeleteUserTierQuery,
	deleteUsersMarked:            sqliteDeleteUsersMarkedQuery,
	deleteUsersProvisioned:       sqliteDeleteUsersProvisionedQuery,
	selectTopicPerms:             sqliteSelectTopicPermsQuery,
	selectUserAllAccess:          sqliteSelectUserAllAccessQuery,
	selectUserAccess:             sqliteSelectUserAccessQuery,
	selectUserReservations:       sqliteSelectUserReservationsQuery,
	selectUserReservationsCount:  sqliteSelectUserReservationsCountQuery,
	selectUserReservationsOwner:  sqliteSelectUserReservationsOwnerQuery,
	selectUserHasReservation:     sqliteSelectUserHasReservationQuery,
	selectOtherAccessCount:       sqliteSelectOtherAccessCountQuery,
	upsertUserAccess:             sqliteUpsertUserAccessQuery,
	deleteUserAccess:             sqliteDeleteUserAccessQuery,
	deleteUserAccessProvisioned:  sqliteDeleteUserAccessProvisionedQuery,
	deleteTopicAccess:            sqliteDeleteTopicAccessQuery,
	deleteAllAccess:              sqliteDeleteAllAccessQuery,
	selectToken:                  sqliteSelectTokenQuery,
	selectTokens:                 sqliteSelectTokensQuery,
	selectTokenCount:             sqliteSelectTokenCountQuery,
	selectAllProvisionedTokens:   sqliteSelectAllProvisionedTokensQuery,
	upsertToken:                  sqliteUpsertTokenQuery,
	updateToken:                  sqliteUpdateTokenQuery,
	updateTokenLastAccess:        sqliteUpdateTokenLastAccessQuery,
	deleteToken:                  sqliteDeleteTokenQuery,
	deleteProvisionedToken:       sqliteDeleteProvisionedTokenQuery,
	deleteAllProvisionedTokens:   sqliteDeleteAllProvisionedTokensQuery,
	deleteAllToken:               sqliteDeleteAllTokenQuery,
	deleteExpiredTokens:          sqliteDeleteExpiredTokensQuery,
	deleteExcessTokens:           sqliteDeleteExcessTokensQuery,
	insertTier:                   sqliteInsertTierQuery,
	selectTiers:                  sqliteSelectTiersQuery,
	selectTierByCode:             sqliteSelectTierByCodeQuery,
	selectTierByPriceID:          sqliteSelectTierByPriceIDQuery,
	updateTier:                   sqliteUpdateTierQuery,
	deleteTier:                   sqliteDeleteTierQuery,
	selectPhoneNumbers:           sqliteSelectPhoneNumbersQuery,
	insertPhoneNumber:            sqliteInsertPhoneNumberQuery,
	deletePhoneNumber:            sqliteDeletePhoneNumberQuery,
	selectEmails:                 sqliteSelectEmailsQuery,
	insertEmail:                  sqliteInsertEmailQuery,
	deleteEmail:                  sqliteDeleteEmailQuery,
	updateBilling:                sqliteUpdateBillingQuery,
}

// NewSQLiteManager creates a new Manager backed by a SQLite database
func NewSQLiteManager(filename, startupQueries string, config *Config) (*Manager, error) {
	parentDir := filepath.Dir(filename)
	if !util.FileExists(parentDir) {
		return nil, fmt.Errorf("user database directory %s does not exist or is not accessible", parentDir)
	}
	d, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupSQLite(d); err != nil {
		return nil, err
	}
	if err := runSQLiteStartupQueries(d, startupQueries); err != nil {
		return nil, err
	}
	return newManager(db.New(&db.Host{DB: d}, nil), sqliteQueries, config)
}
