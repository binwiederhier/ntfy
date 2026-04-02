package user

import (
	"heckel.io/ntfy/v2/db"
)

// PostgreSQL queries
const (
	// User queries
	postgresSelectUsersQuery = `
		SELECT u.id, u.user_name, u.pass, u.role, u.prefs, u.sync_topic, u.provisioned, u.stats_messages, u.stats_emails, u.stats_calls, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, u.deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.calls_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM "user" u
		LEFT JOIN tier t on t.id = u.tier_id
		ORDER BY
			CASE u.role
				WHEN 'admin' THEN 1
				WHEN 'anonymous' THEN 3
				ELSE 2
			END, u.user_name
	`
	postgresSelectUserByIDQuery = `
		SELECT u.id, u.user_name, u.pass, u.role, u.prefs, u.sync_topic, u.provisioned, u.stats_messages, u.stats_emails, u.stats_calls, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, u.deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.calls_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM "user" u
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE u.id = $1
	`
	postgresSelectUserByNameQuery = `
		SELECT u.id, u.user_name, u.pass, u.role, u.prefs, u.sync_topic, u.provisioned, u.stats_messages, u.stats_emails, u.stats_calls, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, u.deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.calls_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM "user" u
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE user_name = $1
	`
	postgresSelectUserByTokenQuery = `
		SELECT u.id, u.user_name, u.pass, u.role, u.prefs, u.sync_topic, u.provisioned, u.stats_messages, u.stats_emails, u.stats_calls, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, u.deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.calls_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM "user" u
		JOIN user_token tk on u.id = tk.user_id
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE tk.token = $1 AND (tk.expires = 0 OR tk.expires >= $2)
	`
	postgresSelectUserByStripeCustomerIDQuery = `
		SELECT u.id, u.user_name, u.pass, u.role, u.prefs, u.sync_topic, u.provisioned, u.stats_messages, u.stats_emails, u.stats_calls, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, u.deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.calls_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM "user" u
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE u.stripe_customer_id = $1
	`
	postgresSelectUsernamesQuery = `
		SELECT user_name
		FROM "user"
		ORDER BY
			CASE role
				WHEN 'admin' THEN 1
				WHEN 'anonymous' THEN 3
				ELSE 2
			END, user_name
	`
	postgresSelectUserCountQuery          = `SELECT COUNT(*) FROM "user"`
	postgresSelectUserIDFromUsernameQuery = `SELECT id FROM "user" WHERE user_name = $1`
	postgresInsertUserQuery               = `INSERT INTO "user" (id, user_name, pass, role, sync_topic, provisioned, created) VALUES ($1, $2, $3, $4, $5, $6, $7)`
	postgresUpdateUserPassQuery           = `UPDATE "user" SET pass = $1 WHERE user_name = $2`
	postgresUpdateUserRoleQuery           = `UPDATE "user" SET role = $1 WHERE user_name = $2`
	postgresUpdateUserProvisionedQuery    = `UPDATE "user" SET provisioned = $1 WHERE user_name = $2`
	postgresUpdateUserPrefsQuery          = `UPDATE "user" SET prefs = $1 WHERE id = $2`
	postgresUpdateUserStatsQuery          = `UPDATE "user" SET stats_messages = $1, stats_emails = $2, stats_calls = $3 WHERE id = $4`
	postgresUpdateUserStatsResetAllQuery  = `UPDATE "user" SET stats_messages = 0, stats_emails = 0, stats_calls = 0`
	postgresUpdateUserTierQuery           = `UPDATE "user" SET tier_id = (SELECT id FROM tier WHERE code = $1) WHERE user_name = $2`
	postgresUpdateUserDeletedQuery        = `UPDATE "user" SET deleted = $1 WHERE id = $2`
	postgresDeleteUserQuery               = `DELETE FROM "user" WHERE user_name = $1`
	postgresDeleteUserTierQuery           = `UPDATE "user" SET tier_id = null WHERE user_name = $1`
	postgresDeleteUsersMarkedQuery        = `DELETE FROM "user" WHERE deleted < $1`
	postgresDeleteUsersProvisionedQuery   = `DELETE FROM "user" WHERE provisioned = true`

	// Access queries
	postgresSelectTopicPermsQuery = `
		SELECT read, write
		FROM user_access a
		JOIN "user" u ON u.id = a.user_id
		WHERE (u.user_name = $1 OR u.user_name = $2) AND $3 LIKE a.topic ESCAPE '\'
		ORDER BY u.user_name DESC, LENGTH(a.topic) DESC, CASE WHEN a.write THEN 1 ELSE 0 END DESC
	`
	postgresSelectUserAllAccessQuery = `
		SELECT user_id, topic, read, write, provisioned
		FROM user_access
		ORDER BY LENGTH(topic) DESC, CASE WHEN write THEN 1 ELSE 0 END DESC, CASE WHEN read THEN 1 ELSE 0 END DESC, topic
	`
	postgresSelectUserAccessQuery = `
		SELECT topic, read, write, provisioned
		FROM user_access
		WHERE user_id = (SELECT id FROM "user" WHERE user_name = $1)
		ORDER BY LENGTH(topic) DESC, CASE WHEN write THEN 1 ELSE 0 END DESC, CASE WHEN read THEN 1 ELSE 0 END DESC, topic
	`
	postgresSelectUserReservationsQuery = `
		SELECT a_user.topic, a_user.read, a_user.write, a_everyone.read AS everyone_read, a_everyone.write AS everyone_write
		FROM user_access a_user
		LEFT JOIN  user_access a_everyone ON a_user.topic = a_everyone.topic AND a_everyone.user_id = (SELECT id FROM "user" WHERE user_name = $1)
		WHERE a_user.user_id = a_user.owner_user_id
		  AND a_user.owner_user_id = (SELECT id FROM "user" WHERE user_name = $2)
		ORDER BY a_user.topic
	`
	postgresSelectUserReservationsCountQuery = `
		SELECT COUNT(*)
		FROM user_access
		WHERE user_id = owner_user_id
		  AND owner_user_id = (SELECT id FROM "user" WHERE user_name = $1)
	`
	postgresSelectUserReservationsOwnerQuery = `
		SELECT owner_user_id
		FROM user_access
		WHERE topic = $1
		  AND user_id = owner_user_id
	`
	postgresSelectUserHasReservationQuery = `
		SELECT COUNT(*)
		FROM user_access
		WHERE user_id = owner_user_id
		  AND owner_user_id = (SELECT id FROM "user" WHERE user_name = $1)
		  AND topic = $2
	`
	postgresSelectOtherAccessCountQuery = `
		SELECT COUNT(*)
		FROM user_access
		WHERE (topic = $1 OR $2 LIKE topic ESCAPE '\')
		  AND (owner_user_id IS NULL OR owner_user_id != (SELECT id FROM "user" WHERE user_name = $3))
	`
	postgresUpsertUserAccessQuery = `
		INSERT INTO user_access (user_id, topic, read, write, owner_user_id, provisioned)
		VALUES (
			(SELECT id FROM "user" WHERE user_name = $1),
			$2,
			$3,
			$4,
			CASE WHEN $5 = '' THEN NULL ELSE (SELECT id FROM "user" WHERE user_name = $6) END,
			$7
		)
		ON CONFLICT (user_id, topic)
		DO UPDATE SET read=excluded.read, write=excluded.write, owner_user_id=excluded.owner_user_id, provisioned=excluded.provisioned
	`
	postgresDeleteUserAccessQuery = `
		DELETE FROM user_access
		WHERE user_id = (SELECT id FROM "user" WHERE user_name = $1)
		   OR owner_user_id = (SELECT id FROM "user" WHERE user_name = $2)
	`
	postgresDeleteUserAccessProvisionedQuery = `DELETE FROM user_access WHERE provisioned = true`
	postgresDeleteTopicAccessQuery           = `
		DELETE FROM user_access
	   	WHERE (user_id = (SELECT id FROM "user" WHERE user_name = $1) OR owner_user_id = (SELECT id FROM "user" WHERE user_name = $2))
	   	  AND topic = $3
  	`
	postgresDeleteAllAccessQuery = `DELETE FROM user_access`

	// Token queries
	postgresSelectTokenQuery                = `SELECT token, label, last_access, last_origin, expires, provisioned FROM user_token WHERE user_id = $1 AND token = $2`
	postgresSelectTokensQuery               = `SELECT token, label, last_access, last_origin, expires, provisioned FROM user_token WHERE user_id = $1`
	postgresSelectTokenCountQuery           = `SELECT COUNT(*) FROM user_token WHERE user_id = $1`
	postgresSelectAllProvisionedTokensQuery = `SELECT token, label, last_access, last_origin, expires, provisioned FROM user_token WHERE provisioned = true`
	postgresUpsertTokenQuery                = `
		INSERT INTO user_token (user_id, token, label, last_access, last_origin, expires, provisioned)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, token)
		DO UPDATE SET label = excluded.label, expires = excluded.expires, provisioned = excluded.provisioned
	`
	postgresUpdateTokenQuery                = `UPDATE user_token SET label = $1, expires = $2 WHERE user_id = $3 AND token = $4`
	postgresUpdateTokenLastAccessQuery      = `UPDATE user_token SET last_access = $1, last_origin = $2 WHERE token = $3`
	postgresDeleteTokenQuery                = `DELETE FROM user_token WHERE user_id = $1 AND token = $2`
	postgresDeleteProvisionedTokenQuery     = `DELETE FROM user_token WHERE token = $1`
	postgresDeleteAllProvisionedTokensQuery = `DELETE FROM user_token WHERE provisioned = true`
	postgresDeleteAllTokenQuery             = `DELETE FROM user_token WHERE user_id = $1`
	postgresDeleteExpiredTokensQuery        = `DELETE FROM user_token WHERE expires > 0 AND expires < $1`
	postgresDeleteExcessTokensQuery         = `
		DELETE FROM user_token
		WHERE user_id = $1
		  AND (user_id, token) NOT IN (
			SELECT user_id, token
			FROM user_token
			WHERE user_id = $2
			ORDER BY expires DESC
			LIMIT $3
		)
	`

	// Tier queries
	postgresInsertTierQuery = `
		INSERT INTO tier (id, code, name, messages_limit, messages_expiry_duration, emails_limit, calls_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`
	postgresUpdateTierQuery = `
		UPDATE tier
		SET name = $1, messages_limit = $2, messages_expiry_duration = $3, emails_limit = $4, calls_limit = $5, reservations_limit = $6, attachment_file_size_limit = $7, attachment_total_size_limit = $8, attachment_expiry_duration = $9, attachment_bandwidth_limit = $10, stripe_monthly_price_id = $11, stripe_yearly_price_id = $12
		WHERE code = $13
	`
	postgresSelectTiersQuery = `
		SELECT id, code, name, messages_limit, messages_expiry_duration, emails_limit, calls_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id
		FROM tier
	`
	postgresSelectTierByCodeQuery = `
		SELECT id, code, name, messages_limit, messages_expiry_duration, emails_limit, calls_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id
		FROM tier
		WHERE code = $1
	`
	postgresSelectTierByPriceIDQuery = `
		SELECT id, code, name, messages_limit, messages_expiry_duration, emails_limit, calls_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id
		FROM tier
		WHERE (stripe_monthly_price_id = $1 OR stripe_yearly_price_id = $2)
	`
	postgresDeleteTierQuery = `DELETE FROM tier WHERE code = $1`

	// Phone queries
	postgresSelectPhoneNumbersQuery = `SELECT phone_number FROM user_phone WHERE user_id = $1`
	postgresInsertPhoneNumberQuery  = `INSERT INTO user_phone (user_id, phone_number) VALUES ($1, $2)`
	postgresDeletePhoneNumberQuery  = `DELETE FROM user_phone WHERE user_id = $1 AND phone_number = $2`

	// Email queries
	postgresSelectEmailsQuery = `SELECT email FROM user_email WHERE user_id = $1 ORDER BY email`
	postgresInsertEmailQuery  = `INSERT INTO user_email (user_id, email) VALUES ($1, $2)`
	postgresDeleteEmailQuery  = `DELETE FROM user_email WHERE user_id = $1 AND email = $2`

	// Billing queries
	postgresUpdateBillingQuery = `
		UPDATE "user"
		SET stripe_customer_id = $1, stripe_subscription_id = $2, stripe_subscription_status = $3, stripe_subscription_interval = $4, stripe_subscription_paid_until = $5, stripe_subscription_cancel_at = $6
		WHERE user_name = $7
	`
)

// NewPostgresManager creates a new Manager backed by a PostgreSQL database using an existing connection pool.
var postgresQueries = queries{
	selectUserByID:               postgresSelectUserByIDQuery,
	selectUserByName:             postgresSelectUserByNameQuery,
	selectUserByToken:            postgresSelectUserByTokenQuery,
	selectUserByStripeCustomerID: postgresSelectUserByStripeCustomerIDQuery,
	selectUsernames:              postgresSelectUsernamesQuery,
	selectUsers:                  postgresSelectUsersQuery,
	selectUserCount:              postgresSelectUserCountQuery,
	selectUserIDFromUsername:     postgresSelectUserIDFromUsernameQuery,
	insertUser:                   postgresInsertUserQuery,
	updateUserPass:               postgresUpdateUserPassQuery,
	updateUserRole:               postgresUpdateUserRoleQuery,
	updateUserProvisioned:        postgresUpdateUserProvisionedQuery,
	updateUserPrefs:              postgresUpdateUserPrefsQuery,
	updateUserStats:              postgresUpdateUserStatsQuery,
	updateUserStatsResetAll:      postgresUpdateUserStatsResetAllQuery,
	updateUserTier:               postgresUpdateUserTierQuery,
	updateUserDeleted:            postgresUpdateUserDeletedQuery,
	deleteUser:                   postgresDeleteUserQuery,
	deleteUserTier:               postgresDeleteUserTierQuery,
	deleteUsersMarked:            postgresDeleteUsersMarkedQuery,
	deleteUsersProvisioned:       postgresDeleteUsersProvisionedQuery,
	selectTopicPerms:             postgresSelectTopicPermsQuery,
	selectUserAllAccess:          postgresSelectUserAllAccessQuery,
	selectUserAccess:             postgresSelectUserAccessQuery,
	selectUserReservations:       postgresSelectUserReservationsQuery,
	selectUserReservationsCount:  postgresSelectUserReservationsCountQuery,
	selectUserReservationsOwner:  postgresSelectUserReservationsOwnerQuery,
	selectUserHasReservation:     postgresSelectUserHasReservationQuery,
	selectOtherAccessCount:       postgresSelectOtherAccessCountQuery,
	upsertUserAccess:             postgresUpsertUserAccessQuery,
	deleteUserAccess:             postgresDeleteUserAccessQuery,
	deleteUserAccessProvisioned:  postgresDeleteUserAccessProvisionedQuery,
	deleteTopicAccess:            postgresDeleteTopicAccessQuery,
	deleteAllAccess:              postgresDeleteAllAccessQuery,
	selectToken:                  postgresSelectTokenQuery,
	selectTokens:                 postgresSelectTokensQuery,
	selectTokenCount:             postgresSelectTokenCountQuery,
	selectAllProvisionedTokens:   postgresSelectAllProvisionedTokensQuery,
	upsertToken:                  postgresUpsertTokenQuery,
	updateToken:                  postgresUpdateTokenQuery,
	updateTokenLastAccess:        postgresUpdateTokenLastAccessQuery,
	deleteToken:                  postgresDeleteTokenQuery,
	deleteProvisionedToken:       postgresDeleteProvisionedTokenQuery,
	deleteAllProvisionedTokens:   postgresDeleteAllProvisionedTokensQuery,
	deleteAllToken:               postgresDeleteAllTokenQuery,
	deleteExpiredTokens:          postgresDeleteExpiredTokensQuery,
	deleteExcessTokens:           postgresDeleteExcessTokensQuery,
	insertTier:                   postgresInsertTierQuery,
	selectTiers:                  postgresSelectTiersQuery,
	selectTierByCode:             postgresSelectTierByCodeQuery,
	selectTierByPriceID:          postgresSelectTierByPriceIDQuery,
	updateTier:                   postgresUpdateTierQuery,
	deleteTier:                   postgresDeleteTierQuery,
	selectPhoneNumbers:           postgresSelectPhoneNumbersQuery,
	insertPhoneNumber:            postgresInsertPhoneNumberQuery,
	deletePhoneNumber:            postgresDeletePhoneNumberQuery,
	selectEmails:                 postgresSelectEmailsQuery,
	insertEmail:                  postgresInsertEmailQuery,
	deleteEmail:                  postgresDeleteEmailQuery,
	updateBilling:                postgresUpdateBillingQuery,
}

// NewPostgresManager creates a new Manager backed by a PostgreSQL database
func NewPostgresManager(d *db.DB, config *Config) (*Manager, error) {
	if err := setupPostgres(d.Primary()); err != nil {
		return nil, err
	}
	return newManager(d, postgresQueries, config)
}
