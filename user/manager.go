// Package user deals with authentication and authorization against topics
package user

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/payments"
	"heckel.io/ntfy/v2/util"
)

const (
	tierIDPrefix                    = "ti_"
	tierIDLength                    = 8
	syncTopicPrefix                 = "st_"
	syncTopicLength                 = 16
	userIDPrefix                    = "u_"
	userIDLength                    = 12
	userAuthIntentionalSlowDownHash = "$2a$10$YFCQvqQDwIIwnJM1xkAYOeih0dg17UVGanaTStnrSzC8NCWxcLDwy" // Cost should match DefaultUserPasswordBcryptCost
	userHardDeleteAfterDuration     = 7 * 24 * time.Hour
	tokenPrefix                     = "tk_"
	tokenLength                     = 32
	tokenMaxCount                   = 60 // Only keep this many tokens in the table per user
	tag                             = "user_manager"
)

// Default constants that may be overridden by configs
const (
	DefaultUserStatsQueueWriterInterval = 33 * time.Second
	DefaultUserPasswordBcryptCost       = 10
)

var (
	errNoTokenProvided    = errors.New("no token provided")
	errTopicOwnedByOthers = errors.New("topic owned by others")
	errNoRows             = errors.New("no rows found")
)

// Manager handles user authentication, authorization, and management
type Manager struct {
	config     *Config
	db         *db.DB
	queries    queries
	statsQueue map[string]*Stats       // "Queue" to asynchronously write user stats to the database (UserID -> Stats)
	tokenQueue map[string]*TokenUpdate // "Queue" to asynchronously write token access stats to the database (Token ID -> TokenUpdate)
	mu         sync.Mutex
}

var _ Auther = (*Manager)(nil)

func newManager(d *db.DB, queries queries, config *Config) (*Manager, error) {
	if config.BcryptCost <= 0 {
		config.BcryptCost = DefaultUserPasswordBcryptCost
	}
	if config.QueueWriterInterval.Seconds() <= 0 {
		config.QueueWriterInterval = DefaultUserStatsQueueWriterInterval
	}
	manager := &Manager{
		config:     config,
		db:         d,
		statsQueue: make(map[string]*Stats),
		tokenQueue: make(map[string]*TokenUpdate),
		queries:    queries,
	}
	if err := manager.maybeProvisionUsersAccessAndTokens(); err != nil {
		return nil, err
	}
	go manager.asyncQueueWriter(manager.config.QueueWriterInterval)
	return manager, nil
}

// Authenticate checks username and password and returns a User if correct, and the user has not been
// marked as deleted. The method returns in constant-ish time, regardless of whether the user exists or
// the password is correct or incorrect.
func (a *Manager) Authenticate(username, password string) (*User, error) {
	if username == Everyone {
		return nil, ErrUnauthenticated
	}
	user, err := a.User(username)
	if err != nil {
		log.Tag(tag).Field("user_name", username).Err(err).Trace("Authentication of user failed (1)")
		bcrypt.CompareHashAndPassword([]byte(userAuthIntentionalSlowDownHash), []byte("intentional slow-down to avoid timing attacks"))
		return nil, ErrUnauthenticated
	} else if user.Deleted {
		log.Tag(tag).Field("user_name", username).Trace("Authentication of user failed (2): user marked deleted")
		bcrypt.CompareHashAndPassword([]byte(userAuthIntentionalSlowDownHash), []byte("intentional slow-down to avoid timing attacks"))
		return nil, ErrUnauthenticated
	} else if err := bcrypt.CompareHashAndPassword([]byte(user.Hash), []byte(password)); err != nil {
		log.Tag(tag).Field("user_name", username).Err(err).Trace("Authentication of user failed (3)")
		return nil, ErrUnauthenticated
	}
	return user, nil
}

// AuthenticateToken checks if the token exists and returns the associated User if it does.
// The method sets the User.Token value to the token that was used for authentication.
func (a *Manager) AuthenticateToken(token string) (*User, error) {
	if len(token) != tokenLength {
		return nil, ErrUnauthenticated
	}
	user, err := a.userByToken(token)
	if err != nil {
		log.Tag(tag).Field("token", token).Err(err).Trace("Authentication of token failed")
		return nil, ErrUnauthenticated
	}
	user.Token = token
	return user, nil
}

// AddUser adds a user with the given username, password and role
func (a *Manager) AddUser(username, password string, role Role, hashed bool) error {
	hash, err := a.maybeHashPassword(password, hashed)
	if err != nil {
		return err
	}
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		return a.addUserTx(tx, username, hash, role, false)
	})
}

// addUserTx adds a user with the given username, password hash and role to the database
func (a *Manager) addUserTx(tx *sql.Tx, username, hash string, role Role, provisioned bool) error {
	if !AllowedUsername(username) || !AllowedRole(role) {
		return ErrInvalidArgument
	}
	userID := util.RandomStringPrefix(userIDPrefix, userIDLength)
	syncTopic := util.RandomStringPrefix(syncTopicPrefix, syncTopicLength)
	now := time.Now().Unix()
	if _, err := tx.Exec(a.queries.insertUser, userID, username, hash, string(role), syncTopic, provisioned, now); err != nil {
		if isUniqueConstraintError(err) {
			return ErrUserExists
		}
		return err
	}
	return nil
}

// RemoveUser deletes the user with the given username. The function returns nil on success, even
// if the user did not exist in the first place.
func (a *Manager) RemoveUser(username string) error {
	if err := a.CanChangeUser(username); err != nil {
		return err
	}
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		return a.removeUserTx(tx, username)
	})
}

// removeUserTx deletes the user with the given username
func (a *Manager) removeUserTx(tx *sql.Tx, username string) error {
	if !AllowedUsername(username) {
		return ErrInvalidArgument
	}
	// Rows in user_access, user_token, etc. are deleted via foreign keys
	if _, err := tx.Exec(a.queries.deleteUser, username); err != nil {
		return err
	}
	return nil
}

// MarkUserRemoved sets the deleted flag on the user, and deletes all access tokens. This prevents
// successful auth via Authenticate. A background process will delete the user at a later date.
func (a *Manager) MarkUserRemoved(user *User) error {
	if !AllowedUsername(user.Name) {
		return ErrInvalidArgument
	}
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		if err := a.resetUserAccessTx(tx, user.Name); err != nil {
			return err
		}
		if _, err := tx.Exec(a.queries.deleteAllToken, user.ID); err != nil {
			return err
		}
		deletedTime := time.Now().Add(userHardDeleteAfterDuration).Unix()
		if _, err := tx.Exec(a.queries.updateUserDeleted, deletedTime, user.ID); err != nil {
			return err
		}
		return nil
	})
}

// RemoveDeletedUsers deletes all users that have been marked deleted
func (a *Manager) RemoveDeletedUsers() error {
	if _, err := a.db.Exec(a.queries.deleteUsersMarked, time.Now().Unix()); err != nil {
		return err
	}
	return nil
}

// ChangePassword changes a user's password
func (a *Manager) ChangePassword(username, password string, hashed bool) error {
	if err := a.CanChangeUser(username); err != nil {
		return err
	}
	hash, err := a.maybeHashPassword(password, hashed)
	if err != nil {
		return err
	}
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		return a.changePasswordHashTx(tx, username, hash)
	})
}

// changePasswordHashTx changes a user's password hash in the database
func (a *Manager) changePasswordHashTx(tx *sql.Tx, username, hash string) error {
	if _, err := tx.Exec(a.queries.updateUserPass, hash, username); err != nil {
		return err
	}
	return nil
}

// ChangeRole changes a user's role. When a role is changed from RoleUser to RoleAdmin,
// all existing access control entries (Grant) are removed, since they are no longer needed.
func (a *Manager) ChangeRole(username string, role Role) error {
	if err := a.CanChangeUser(username); err != nil {
		return err
	}
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		return a.changeRoleTx(tx, username, role)
	})
}

// changeRoleTx changes a user's role
func (a *Manager) changeRoleTx(tx *sql.Tx, username string, role Role) error {
	if !AllowedUsername(username) || !AllowedRole(role) {
		return ErrInvalidArgument
	}
	if _, err := tx.Exec(a.queries.updateUserRole, string(role), username); err != nil {
		return err
	}
	// If changing to admin, remove all access entries
	if role == RoleAdmin {
		if err := a.resetUserAccessTx(tx, username); err != nil {
			return err
		}
	}
	return nil
}

// CanChangeUser checks if the user with the given username can be changed.
// This is used to prevent changes to provisioned users, which are defined in the config file.
func (a *Manager) CanChangeUser(username string) error {
	user, err := a.User(username)
	if err != nil {
		return err
	} else if user.Provisioned {
		return ErrProvisionedUserChange
	}
	return nil
}

// changeProvisionedTx changes the provisioned status of a user
func (a *Manager) changeProvisionedTx(tx *sql.Tx, username string, provisioned bool) error {
	if _, err := tx.Exec(a.queries.updateUserProvisioned, provisioned, username); err != nil {
		return err
	}
	return nil
}

// ChangeSettings persists the user settings
func (a *Manager) ChangeSettings(userID string, prefs *Prefs) error {
	b, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	if _, err := a.db.Exec(a.queries.updateUserPrefs, string(b), userID); err != nil {
		return err
	}
	return nil
}

// ChangeTier changes a user's tier using the tier code. This function does not delete reservations, messages,
// or attachments, even if the new tier has lower limits in this regard. That has to be done elsewhere.
func (a *Manager) ChangeTier(username, tier string) error {
	if !AllowedUsername(username) {
		return ErrInvalidArgument
	}
	t, err := a.Tier(tier)
	if err != nil {
		return err
	}
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		if err := a.checkReservationsLimitTx(tx, username, t.ReservationLimit); err != nil {
			return err
		}
		if _, err := tx.Exec(a.queries.updateUserTier, tier, username); err != nil {
			return err
		}
		return nil
	})
}

// ResetTier removes the tier from the given user
func (a *Manager) ResetTier(username string) error {
	if !AllowedUsername(username) && username != Everyone && username != "" {
		return ErrInvalidArgument
	}
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		if err := a.checkReservationsLimitTx(tx, username, 0); err != nil {
			return err
		}
		if _, err := tx.Exec(a.queries.deleteUserTier, username); err != nil {
			return err
		}
		return nil
	})
}

func (a *Manager) checkReservationsLimitTx(tx *sql.Tx, username string, reservationsLimit int64) error {
	u, err := a.userTx(tx, username)
	if err != nil {
		return err
	}
	if u.Tier != nil && reservationsLimit < u.Tier.ReservationLimit {
		reservations, err := a.reservationsTx(tx, username)
		if err != nil {
			return err
		} else if int64(len(reservations)) > reservationsLimit {
			return ErrTooManyReservations
		}
	}
	return nil
}

// ResetStats resets all user stats in the user database. This touches all users.
func (a *Manager) ResetStats() error {
	a.mu.Lock() // Includes database query to avoid races!
	defer a.mu.Unlock()
	if _, err := a.db.Exec(a.queries.updateUserStatsResetAll); err != nil {
		return err
	}
	a.statsQueue = make(map[string]*Stats)
	return nil
}

// EnqueueUserStats adds the user to a queue which writes out user stats (messages, emails, ..) in
// batches at a regular interval
func (a *Manager) EnqueueUserStats(userID string, stats *Stats) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statsQueue[userID] = stats
}

func (a *Manager) asyncQueueWriter(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		if err := a.writeUserStatsQueue(); err != nil {
			log.Tag(tag).Err(err).Warn("Writing user stats queue failed")
		}
		if err := a.writeTokenUpdateQueue(); err != nil {
			log.Tag(tag).Err(err).Warn("Writing token update queue failed")
		}
	}
}

func (a *Manager) writeUserStatsQueue() error {
	a.mu.Lock()
	if len(a.statsQueue) == 0 {
		a.mu.Unlock()
		log.Tag(tag).Trace("No user stats updates to commit")
		return nil
	}
	statsQueue := a.statsQueue
	a.statsQueue = make(map[string]*Stats)
	a.mu.Unlock()

	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		log.Tag(tag).Debug("Writing user stats queue for %d user(s)", len(statsQueue))
		for userID, update := range statsQueue {
			log.
				Tag(tag).
				Fields(log.Context{
					"user_id":        userID,
					"messages_count": update.Messages,
					"emails_count":   update.Emails,
					"calls_count":    update.Calls,
				}).
				Trace("Updating stats for user %s", userID)
			if _, err := tx.Exec(a.queries.updateUserStats, update.Messages, update.Emails, update.Calls, userID); err != nil {
				return err
			}
		}
		return nil
	})
}

// User returns the user with the given username if it exists, or ErrUserNotFound otherwise
func (a *Manager) User(username string) (*User, error) {
	return a.userTx(a.db, username)
}

func (a *Manager) userTx(tx db.Querier, username string) (*User, error) {
	rows, err := tx.Query(a.queries.selectUserByName, username)
	if err != nil {
		return nil, err
	}
	return a.readUser(rows)
}

// UserByID returns the user with the given ID if it exists, or ErrUserNotFound otherwise
func (a *Manager) UserByID(id string) (*User, error) {
	rows, err := a.db.Query(a.queries.selectUserByID, id)
	if err != nil {
		return nil, err
	}
	return a.readUser(rows)
}

// userByToken returns the user with the given token if it exists and is not expired, or ErrUserNotFound otherwise
func (a *Manager) userByToken(token string) (*User, error) {
	rows, err := a.db.Query(a.queries.selectUserByToken, token, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return a.readUser(rows)
}

// UserByStripeCustomer returns the user with the given Stripe customer ID if it exists, or ErrUserNotFound otherwise
func (a *Manager) UserByStripeCustomer(customerID string) (*User, error) {
	rows, err := a.db.Query(a.queries.selectUserByStripeCustomerID, customerID)
	if err != nil {
		return nil, err
	}
	return a.readUser(rows)
}

// Users returns a list of users. It loads all users in a single query
// rather than one query per user to avoid N+1 performance issues.
func (a *Manager) Users() ([]*User, error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectUsers)
	if err != nil {
		return nil, err
	}
	return a.readUsers(rows)
}

// UsersCount returns the number of users in the database
func (a *Manager) UsersCount() (int64, error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectUserCount)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errNoRows
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (a *Manager) readUser(rows *sql.Rows) (*User, error) {
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrUserNotFound
	}
	user, err := a.scanUser(rows)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (a *Manager) readUsers(rows *sql.Rows) ([]*User, error) {
	defer rows.Close()
	users := make([]*User, 0)
	for rows.Next() {
		user, err := a.scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (a *Manager) scanUser(rows *sql.Rows) (*User, error) {
	var id, username, hash, role, prefs, syncTopic string
	var provisioned bool
	var stripeCustomerID, stripeSubscriptionID, stripeSubscriptionStatus, stripeSubscriptionInterval, stripeMonthlyPriceID, stripeYearlyPriceID, tierID, tierCode, tierName sql.NullString
	var messages, emails, calls int64
	var messagesLimit, messagesExpiryDuration, emailsLimit, callsLimit, reservationsLimit, attachmentFileSizeLimit, attachmentTotalSizeLimit, attachmentExpiryDuration, attachmentBandwidthLimit, stripeSubscriptionPaidUntil, stripeSubscriptionCancelAt, deleted sql.NullInt64
	if err := rows.Scan(&id, &username, &hash, &role, &prefs, &syncTopic, &provisioned, &messages, &emails, &calls, &stripeCustomerID, &stripeSubscriptionID, &stripeSubscriptionStatus, &stripeSubscriptionInterval, &stripeSubscriptionPaidUntil, &stripeSubscriptionCancelAt, &deleted, &tierID, &tierCode, &tierName, &messagesLimit, &messagesExpiryDuration, &emailsLimit, &callsLimit, &reservationsLimit, &attachmentFileSizeLimit, &attachmentTotalSizeLimit, &attachmentExpiryDuration, &attachmentBandwidthLimit, &stripeMonthlyPriceID, &stripeYearlyPriceID); err != nil {
		return nil, err
	} else if err := rows.Err(); err != nil {
		return nil, err
	}
	user := &User{
		ID:          id,
		Name:        username,
		Hash:        hash,
		Role:        Role(role),
		Prefs:       &Prefs{},
		SyncTopic:   syncTopic,
		Provisioned: provisioned,
		Stats: &Stats{
			Messages: messages,
			Emails:   emails,
			Calls:    calls,
		},
		Billing: &Billing{
			StripeCustomerID:            stripeCustomerID.String,                                            // May be empty
			StripeSubscriptionID:        stripeSubscriptionID.String,                                        // May be empty
			StripeSubscriptionStatus:    payments.SubscriptionStatus(stripeSubscriptionStatus.String),       // May be empty
			StripeSubscriptionInterval:  payments.PriceRecurringInterval(stripeSubscriptionInterval.String), // May be empty
			StripeSubscriptionPaidUntil: time.Unix(stripeSubscriptionPaidUntil.Int64, 0),                    // May be zero
			StripeSubscriptionCancelAt:  time.Unix(stripeSubscriptionCancelAt.Int64, 0),                     // May be zero
		},
		Deleted: deleted.Valid,
	}
	if err := json.Unmarshal([]byte(prefs), user.Prefs); err != nil {
		return nil, err
	}
	if tierCode.Valid {
		// See readTier() when this is changed!
		user.Tier = &Tier{
			ID:                       tierID.String,
			Code:                     tierCode.String,
			Name:                     tierName.String,
			MessageLimit:             messagesLimit.Int64,
			MessageExpiryDuration:    time.Duration(messagesExpiryDuration.Int64) * time.Second,
			EmailLimit:               emailsLimit.Int64,
			CallLimit:                callsLimit.Int64,
			ReservationLimit:         reservationsLimit.Int64,
			AttachmentFileSizeLimit:  attachmentFileSizeLimit.Int64,
			AttachmentTotalSizeLimit: attachmentTotalSizeLimit.Int64,
			AttachmentExpiryDuration: time.Duration(attachmentExpiryDuration.Int64) * time.Second,
			AttachmentBandwidthLimit: attachmentBandwidthLimit.Int64,
			StripeMonthlyPriceID:     stripeMonthlyPriceID.String, // May be empty
			StripeYearlyPriceID:      stripeYearlyPriceID.String,  // May be empty
		}
	}
	return user, nil
}

func (a *Manager) maybeHashPassword(password string, hashed bool) (string, error) {
	if hashed {
		if err := ValidPasswordHash(password, a.config.BcryptCost); err != nil {
			return "", err
		}
		return password, nil
	}
	return hashPassword(password, a.config.BcryptCost)
}

// Authorize returns nil if the given user has access to the given topic using the desired
// permission. The user param may be nil to signal an anonymous user.
func (a *Manager) Authorize(user *User, topic string, perm Permission) error {
	if user != nil && user.Role == RoleAdmin {
		return nil // Admin can do everything
	}
	username := Everyone
	if user != nil {
		username = user.Name
	}
	// Select the read/write permissions for this user/topic combo.
	read, write, found, err := a.authorizeTopicAccess(username, topic)
	if err != nil {
		return err
	} else if !found {
		return a.resolvePerms(a.config.DefaultAccess, perm)
	}
	return a.resolvePerms(NewPermission(read, write), perm)
}

func (a *Manager) resolvePerms(base, perm Permission) error {
	if perm == PermissionRead && base.IsRead() {
		return nil
	} else if perm == PermissionWrite && base.IsWrite() {
		return nil
	}
	return ErrUnauthorized
}

// AllowAccess adds or updates an entry in the access control list for a specific user. It controls
// read/write access to a topic. The parameter topicPattern may include wildcards (*). The ACL entry
// owner may either be a user (username), or the system (empty).
func (a *Manager) AllowAccess(username string, topicPattern string, permission Permission) error {
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		return a.allowAccessTx(tx, username, topicPattern, permission, false)
	})
}

func (a *Manager) allowAccessTx(tx *sql.Tx, username string, topicPattern string, permission Permission, provisioned bool) error {
	if !AllowedUsername(username) && username != Everyone {
		return ErrInvalidArgument
	} else if !AllowedTopicPattern(topicPattern) {
		return ErrInvalidArgument
	}
	_, err := tx.Exec(a.queries.upsertUserAccess, username, toSQLWildcard(topicPattern), permission.IsRead(), permission.IsWrite(), "", "", provisioned)
	return err
}

// ResetAccess removes an access control list entry for a specific username/topic, or (if topic is
// empty) for an entire user. The parameter topicPattern may include wildcards (*).
func (a *Manager) ResetAccess(username string, topicPattern string) error {
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		return a.resetAccessTx(tx, username, topicPattern)
	})
}

func (a *Manager) resetAccessTx(tx *sql.Tx, username string, topicPattern string) error {
	if !AllowedUsername(username) && username != Everyone && username != "" {
		return ErrInvalidArgument
	} else if !AllowedTopicPattern(topicPattern) && topicPattern != "" {
		return ErrInvalidArgument
	}
	if username == "" && topicPattern == "" {
		_, err := tx.Exec(a.queries.deleteAllAccess)
		return err
	} else if topicPattern == "" {
		return a.resetUserAccessTx(tx, username)
	}
	return a.resetTopicAccessTx(tx, username, topicPattern)
}

// DefaultAccess returns the default read/write access if no access control entry matches
func (a *Manager) DefaultAccess() Permission {
	return a.config.DefaultAccess
}

// AllowReservation tests if a user may create an access control entry for the given topic.
// If there are any ACL entries that are not owned by the user, an error is returned.
func (a *Manager) AllowReservation(username string, topic string) error {
	if (!AllowedUsername(username) && username != Everyone) || !AllowedTopic(topic) {
		return ErrInvalidArgument
	}
	otherCount, err := a.otherAccessCount(username, topic)
	if err != nil {
		return err
	}
	if otherCount > 0 {
		return errTopicOwnedByOthers
	}
	return nil
}

// authorizeTopicAccess returns the read/write permissions for the given username and topic.
// The found return value indicates whether an ACL entry was found at all.
//
// - The query may return two rows (one for everyone, and one for the user), but prioritizes the user.
// - Furthermore, the query prioritizes more specific permissions (longer!) over more generic ones, e.g. "test*" > "*"
// - It also prioritizes write permissions over read permissions
func (a *Manager) authorizeTopicAccess(usernameOrEveryone, topic string) (read, write, found bool, err error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectTopicPerms, Everyone, usernameOrEveryone, topic)
	if err != nil {
		return false, false, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, false, false, nil
	}
	if err := rows.Scan(&read, &write); err != nil {
		return false, false, false, err
	} else if err := rows.Err(); err != nil {
		return false, false, false, err
	}
	return read, write, true, nil
}

// AllGrants returns all user-specific access control entries, mapped to their respective user IDs
func (a *Manager) AllGrants() (map[string][]Grant, error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectUserAllAccess)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := make(map[string][]Grant, 0)
	for rows.Next() {
		var userID, topic string
		var read, write, provisioned bool
		if err := rows.Scan(&userID, &topic, &read, &write, &provisioned); err != nil {
			return nil, err
		} else if err := rows.Err(); err != nil {
			return nil, err
		}
		if _, ok := grants[userID]; !ok {
			grants[userID] = make([]Grant, 0)
		}
		grants[userID] = append(grants[userID], Grant{
			TopicPattern: fromSQLWildcard(topic),
			Permission:   NewPermission(read, write),
			Provisioned:  provisioned,
		})
	}
	return grants, nil
}

// Grants returns all user-specific access control entries
func (a *Manager) Grants(username string) ([]Grant, error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectUserAccess, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := make([]Grant, 0)
	for rows.Next() {
		var topic string
		var read, write, provisioned bool
		if err := rows.Scan(&topic, &read, &write, &provisioned); err != nil {
			return nil, err
		} else if err := rows.Err(); err != nil {
			return nil, err
		}
		grants = append(grants, Grant{
			TopicPattern: fromSQLWildcard(topic),
			Permission:   NewPermission(read, write),
			Provisioned:  provisioned,
		})
	}
	return grants, nil
}

// AddReservation creates two access control entries for the given topic: one with full read/write
// access for the given user, and one for Everyone with the given permission. Both entries are
// created atomically in a single transaction. If limit is > 0, the reservation count is checked
// inside the transaction and ErrTooManyReservations is returned if the limit would be exceeded.
func (a *Manager) AddReservation(username string, topic string, everyone Permission, limit int64) error {
	if !AllowedUsername(username) || username == Everyone || !AllowedTopic(topic) {
		return ErrInvalidArgument
	}
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		if limit > 0 {
			hasReservation, err := a.hasReservationTx(tx, username, topic)
			if err != nil {
				return err
			}
			if !hasReservation {
				count, err := a.reservationsCountTx(tx, username)
				if err != nil {
					return err
				}
				if count >= limit {
					return ErrTooManyReservations
				}
			}
		}
		if _, err := tx.Exec(a.queries.upsertUserAccess, username, toSQLWildcard(topic), true, true, username, username, false); err != nil {
			return err
		}
		if _, err := tx.Exec(a.queries.upsertUserAccess, Everyone, toSQLWildcard(topic), everyone.IsRead(), everyone.IsWrite(), username, username, false); err != nil {
			return err
		}
		return nil
	})
}

// RemoveReservations deletes the access control entries associated with the given username/topic,
// as well as all entries with Everyone/topic. All deletions are performed atomically in a single
// transaction.
func (a *Manager) RemoveReservations(username string, topics ...string) error {
	if !AllowedUsername(username) || username == Everyone || len(topics) == 0 {
		return ErrInvalidArgument
	}
	for _, topic := range topics {
		if !AllowedTopic(topic) {
			return ErrInvalidArgument
		}
	}
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		for _, topic := range topics {
			if err := a.removeReservationAccessTx(tx, username, topic); err != nil {
				return err
			}
		}
		return nil
	})
}

// Reservations returns all user-owned topics, and the associated everyone-access
func (a *Manager) Reservations(username string) ([]Reservation, error) {
	return a.reservationsTx(a.db.ReadOnly(), username)
}

func (a *Manager) reservationsTx(tx db.Querier, username string) ([]Reservation, error) {
	rows, err := tx.Query(a.queries.selectUserReservations, Everyone, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reservations := make([]Reservation, 0)
	for rows.Next() {
		var topic string
		var ownerRead, ownerWrite bool
		var everyoneRead, everyoneWrite sql.NullBool
		if err := rows.Scan(&topic, &ownerRead, &ownerWrite, &everyoneRead, &everyoneWrite); err != nil {
			return nil, err
		} else if err := rows.Err(); err != nil {
			return nil, err
		}
		reservations = append(reservations, Reservation{
			Topic:    fromSQLWildcard(topic),
			Owner:    NewPermission(ownerRead, ownerWrite),
			Everyone: NewPermission(everyoneRead.Bool, everyoneWrite.Bool),
		})
	}
	return reservations, nil
}

// HasReservation returns true if the given topic access is owned by the user
func (a *Manager) HasReservation(username, topic string) (bool, error) {
	return a.hasReservationTx(a.db, username, topic)
}

func (a *Manager) hasReservationTx(tx db.Querier, username, topic string) (bool, error) {
	rows, err := tx.Query(a.queries.selectUserHasReservation, username, escapeUnderscore(topic))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, errNoRows
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// ReservationsCount returns the number of reservations owned by this user
func (a *Manager) ReservationsCount(username string) (int64, error) {
	return a.reservationsCountTx(a.db, username)
}

func (a *Manager) reservationsCountTx(tx db.Querier, username string) (int64, error) {
	rows, err := tx.Query(a.queries.selectUserReservationsCount, username)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errNoRows
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ReservationOwner returns user ID of the user that owns this topic, or an empty string if it's not owned by anyone
func (a *Manager) ReservationOwner(topic string) (string, error) {
	rows, err := a.db.Query(a.queries.selectUserReservationsOwner, escapeUnderscore(topic))
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", nil
	}
	var ownerUserID string
	if err := rows.Scan(&ownerUserID); err != nil {
		return "", err
	}
	return ownerUserID, nil
}

// RemoveExcessReservations removes reservations that exceed the given limit for the user.
// It returns the list of topics whose reservations were removed. The read and removal are
// performed atomically in a single transaction to avoid issues with stale replica data.
func (a *Manager) RemoveExcessReservations(username string, limit int64) ([]string, error) {
	return db.QueryTx(a.db, func(tx *sql.Tx) ([]string, error) {
		reservations, err := a.reservationsTx(tx, username)
		if err != nil {
			return nil, err
		}
		if int64(len(reservations)) <= limit {
			return []string{}, nil
		}
		removedTopics := make([]string, 0)
		for i := int64(len(reservations)) - 1; i >= limit; i-- {
			topic := reservations[i].Topic
			if err := a.removeReservationAccessTx(tx, username, topic); err != nil {
				return nil, err
			}
			removedTopics = append(removedTopics, topic)
		}
		return removedTopics, nil
	})
}

// otherAccessCount returns the number of access entries for the given topic that are not owned by the user
func (a *Manager) otherAccessCount(username, topic string) (int, error) {
	rows, err := a.db.Query(a.queries.selectOtherAccessCount, escapeUnderscore(topic), escapeUnderscore(topic), username)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errNoRows
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (a *Manager) removeReservationAccessTx(tx *sql.Tx, username, topic string) error {
	if err := a.resetTopicAccessTx(tx, username, topic); err != nil {
		return err
	}
	return a.resetTopicAccessTx(tx, Everyone, topic)
}

func (a *Manager) resetUserAccessTx(tx *sql.Tx, username string) error {
	if !AllowedUsername(username) && username != Everyone {
		return ErrInvalidArgument
	}
	_, err := tx.Exec(a.queries.deleteUserAccess, username, username)
	return err
}

func (a *Manager) resetTopicAccessTx(tx *sql.Tx, username, topicPattern string) error {
	if !AllowedUsername(username) && username != Everyone && username != "" {
		return ErrInvalidArgument
	} else if !AllowedTopicPattern(topicPattern) && topicPattern != "" {
		return ErrInvalidArgument
	}
	_, err := tx.Exec(a.queries.deleteTopicAccess, username, username, toSQLWildcard(topicPattern))
	return err
}

// CreateToken generates a random token for the given user and returns it. The token expires
// after a fixed duration unless ChangeToken is called. This function also prunes tokens for the
// given user, if there are too many of them.
func (a *Manager) CreateToken(userID, label string, expires time.Time, origin netip.Addr, provisioned bool) (*Token, error) {
	return db.QueryTx(a.db, func(tx *sql.Tx) (*Token, error) {
		return a.createTokenTx(tx, userID, GenerateToken(), label, time.Now(), origin, expires, tokenMaxCount, provisioned)
	})
}

// createTokenTx creates a new token and prunes excess tokens if the count exceeds maxTokenCount.
// If maxTokenCount is 0, no pruning is performed.
func (a *Manager) createTokenTx(tx *sql.Tx, userID, token, label string, lastAccess time.Time, lastOrigin netip.Addr, expires time.Time, maxTokenCount int, provisioned bool) (*Token, error) {
	if _, err := tx.Exec(a.queries.upsertToken, userID, token, label, lastAccess.Unix(), lastOrigin.String(), expires.Unix(), provisioned); err != nil {
		return nil, err
	}
	if maxTokenCount > 0 {
		var tokenCount int
		if err := tx.QueryRow(a.queries.selectTokenCount, userID).Scan(&tokenCount); err != nil {
			return nil, err
		}
		if tokenCount > maxTokenCount {
			// This pruning logic is done in two queries for efficiency. The SELECT above is a lookup
			// on two indices, whereas the query below is a full table scan.
			if _, err := tx.Exec(a.queries.deleteExcessTokens, userID, userID, maxTokenCount); err != nil {
				return nil, err
			}
		}
	}
	return &Token{
		Value:       token,
		Label:       label,
		LastAccess:  lastAccess,
		LastOrigin:  lastOrigin,
		Expires:     expires,
		Provisioned: provisioned,
	}, nil
}

// ChangeToken updates a token's label and/or expiry date
func (a *Manager) ChangeToken(userID, token string, label *string, expires *time.Time) (*Token, error) {
	if token == "" {
		return nil, errNoTokenProvided
	}
	if err := a.canChangeToken(userID, token); err != nil {
		return nil, err
	}
	t, err := a.Token(userID, token)
	if err != nil {
		return nil, err
	}
	if label != nil {
		t.Label = *label
	}
	if expires != nil {
		t.Expires = *expires
	}
	if _, err := a.db.Exec(a.queries.updateToken, t.Label, t.Expires.Unix(), userID, token); err != nil {
		return nil, err
	}
	return t, nil
}

// RemoveToken deletes the token defined in User.Token
func (a *Manager) RemoveToken(userID, token string) error {
	if err := a.canChangeToken(userID, token); err != nil {
		return err
	}
	if token == "" {
		return errNoTokenProvided
	}
	if _, err := a.db.Exec(a.queries.deleteToken, userID, token); err != nil {
		return err
	}
	return nil
}

// canChangeToken checks if the token can be changed. If the token is provisioned, it cannot be changed.
func (a *Manager) canChangeToken(userID, token string) error {
	t, err := a.Token(userID, token)
	if err != nil {
		return err
	} else if t.Provisioned {
		return ErrProvisionedTokenChange
	}
	return nil
}

// Token returns a specific token for a user
func (a *Manager) Token(userID, token string) (*Token, error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectToken, userID, token)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return a.readToken(rows)
}

// Tokens returns all existing tokens for the user with the given user ID
func (a *Manager) Tokens(userID string) ([]*Token, error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectTokens, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := make([]*Token, 0)
	for {
		token, err := a.readToken(rows)
		if errors.Is(err, ErrTokenNotFound) {
			break
		} else if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func (a *Manager) allProvisionedTokens() ([]*Token, error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectAllProvisionedTokens)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := make([]*Token, 0)
	for {
		token, err := a.readToken(rows)
		if errors.Is(err, ErrTokenNotFound) {
			break
		} else if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// RemoveExpiredTokens deletes all expired tokens from the database
func (a *Manager) RemoveExpiredTokens() error {
	if _, err := a.db.Exec(a.queries.deleteExpiredTokens, time.Now().Unix()); err != nil {
		return err
	}
	return nil
}

// EnqueueTokenUpdate adds the token update to a queue which writes out token access times
// in batches at a regular interval
func (a *Manager) EnqueueTokenUpdate(tokenID string, update *TokenUpdate) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokenQueue[tokenID] = update
}

func (a *Manager) writeTokenUpdateQueue() error {
	a.mu.Lock()
	if len(a.tokenQueue) == 0 {
		a.mu.Unlock()
		log.Tag(tag).Trace("No token updates to commit")
		return nil
	}
	tokenQueue := a.tokenQueue
	a.tokenQueue = make(map[string]*TokenUpdate)
	a.mu.Unlock()

	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		log.Tag(tag).Debug("Writing token update queue for %d token(s)", len(tokenQueue))
		for tokenID, update := range tokenQueue {
			log.Tag(tag).Trace("Updating token %s with last access time %v", tokenID, update.LastAccess.Unix())
			if err := a.updateTokenLastAccessTx(tx, tokenID, update.LastAccess.Unix(), update.LastOrigin.String()); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *Manager) updateTokenLastAccessTx(tx *sql.Tx, token string, lastAccess int64, lastOrigin string) error {
	if _, err := tx.Exec(a.queries.updateTokenLastAccess, lastAccess, lastOrigin, token); err != nil {
		return err
	}
	return nil
}

func (a *Manager) readToken(rows *sql.Rows) (*Token, error) {
	var token, label, lastOrigin string
	var lastAccess, expires int64
	var provisioned bool
	if !rows.Next() {
		return nil, ErrTokenNotFound
	}
	if err := rows.Scan(&token, &label, &lastAccess, &lastOrigin, &expires, &provisioned); err != nil {
		return nil, err
	} else if err := rows.Err(); err != nil {
		return nil, err
	}
	lastOriginIP, err := netip.ParseAddr(lastOrigin)
	if err != nil {
		lastOriginIP = netip.IPv4Unspecified()
	}
	return &Token{
		Value:       token,
		Label:       label,
		LastAccess:  time.Unix(lastAccess, 0),
		LastOrigin:  lastOriginIP,
		Expires:     time.Unix(expires, 0),
		Provisioned: provisioned,
	}, nil
}

// AddTier creates a new tier in the database
func (a *Manager) AddTier(tier *Tier) error {
	if tier.ID == "" {
		tier.ID = util.RandomStringPrefix(tierIDPrefix, tierIDLength)
	}
	if _, err := a.db.Exec(a.queries.insertTier, tier.ID, tier.Code, tier.Name, tier.MessageLimit, int64(tier.MessageExpiryDuration.Seconds()), tier.EmailLimit, tier.CallLimit, tier.ReservationLimit, tier.AttachmentFileSizeLimit, tier.AttachmentTotalSizeLimit, int64(tier.AttachmentExpiryDuration.Seconds()), tier.AttachmentBandwidthLimit, nullString(tier.StripeMonthlyPriceID), nullString(tier.StripeYearlyPriceID)); err != nil {
		return err
	}
	return nil
}

// UpdateTier updates a tier's properties in the database
func (a *Manager) UpdateTier(tier *Tier) error {
	if _, err := a.db.Exec(a.queries.updateTier, tier.Name, tier.MessageLimit, int64(tier.MessageExpiryDuration.Seconds()), tier.EmailLimit, tier.CallLimit, tier.ReservationLimit, tier.AttachmentFileSizeLimit, tier.AttachmentTotalSizeLimit, int64(tier.AttachmentExpiryDuration.Seconds()), tier.AttachmentBandwidthLimit, nullString(tier.StripeMonthlyPriceID), nullString(tier.StripeYearlyPriceID), tier.Code); err != nil {
		return err
	}
	return nil
}

// RemoveTier deletes the tier with the given code
func (a *Manager) RemoveTier(code string) error {
	if !AllowedTier(code) {
		return ErrInvalidArgument
	}
	// This fails if any user has this tier
	if _, err := a.db.Exec(a.queries.deleteTier, code); err != nil {
		return err
	}
	return nil
}

// Tiers returns a list of all Tier structs
func (a *Manager) Tiers() ([]*Tier, error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectTiers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tiers := make([]*Tier, 0)
	for {
		tier, err := a.readTier(rows)
		if errors.Is(err, ErrTierNotFound) {
			break
		} else if err != nil {
			return nil, err
		}
		tiers = append(tiers, tier)
	}
	return tiers, nil
}

// Tier returns a Tier based on the code, or ErrTierNotFound if it does not exist
func (a *Manager) Tier(code string) (*Tier, error) {
	rows, err := a.db.Query(a.queries.selectTierByCode, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return a.readTier(rows)
}

// TierByStripePrice returns a Tier based on the Stripe price ID, or ErrTierNotFound if it does not exist
func (a *Manager) TierByStripePrice(priceID string) (*Tier, error) {
	rows, err := a.db.Query(a.queries.selectTierByPriceID, priceID, priceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return a.readTier(rows)
}

func (a *Manager) readTier(rows *sql.Rows) (*Tier, error) {
	var id, code, name string
	var stripeMonthlyPriceID, stripeYearlyPriceID sql.NullString
	var messagesLimit, messagesExpiryDuration, emailsLimit, callsLimit, reservationsLimit, attachmentFileSizeLimit, attachmentTotalSizeLimit, attachmentExpiryDuration, attachmentBandwidthLimit sql.NullInt64
	if !rows.Next() {
		return nil, ErrTierNotFound
	}
	if err := rows.Scan(&id, &code, &name, &messagesLimit, &messagesExpiryDuration, &emailsLimit, &callsLimit, &reservationsLimit, &attachmentFileSizeLimit, &attachmentTotalSizeLimit, &attachmentExpiryDuration, &attachmentBandwidthLimit, &stripeMonthlyPriceID, &stripeYearlyPriceID); err != nil {
		return nil, err
	} else if err := rows.Err(); err != nil {
		return nil, err
	}
	// When changed, note readUser() as well
	return &Tier{
		ID:                       id,
		Code:                     code,
		Name:                     name,
		MessageLimit:             messagesLimit.Int64,
		MessageExpiryDuration:    time.Duration(messagesExpiryDuration.Int64) * time.Second,
		EmailLimit:               emailsLimit.Int64,
		CallLimit:                callsLimit.Int64,
		ReservationLimit:         reservationsLimit.Int64,
		AttachmentFileSizeLimit:  attachmentFileSizeLimit.Int64,
		AttachmentTotalSizeLimit: attachmentTotalSizeLimit.Int64,
		AttachmentExpiryDuration: time.Duration(attachmentExpiryDuration.Int64) * time.Second,
		AttachmentBandwidthLimit: attachmentBandwidthLimit.Int64,
		StripeMonthlyPriceID:     stripeMonthlyPriceID.String, // May be empty
		StripeYearlyPriceID:      stripeYearlyPriceID.String,  // May be empty
	}, nil
}

// PhoneNumbers returns all phone numbers for the user with the given user ID
func (a *Manager) PhoneNumbers(userID string) ([]string, error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectPhoneNumbers, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	phoneNumbers := make([]string, 0)
	for {
		phoneNumber, err := a.readPhoneNumber(rows)
		if errors.Is(err, ErrPhoneNumberNotFound) {
			break
		} else if err != nil {
			return nil, err
		}
		phoneNumbers = append(phoneNumbers, phoneNumber)
	}
	return phoneNumbers, nil
}

// AddPhoneNumber adds a phone number to the user with the given user ID
func (a *Manager) AddPhoneNumber(userID, phoneNumber string) error {
	if _, err := a.db.Exec(a.queries.insertPhoneNumber, userID, phoneNumber); err != nil {
		if isUniqueConstraintError(err) {
			return ErrPhoneNumberExists
		}
		return err
	}
	return nil
}

// RemovePhoneNumber deletes a phone number from the user with the given user ID
func (a *Manager) RemovePhoneNumber(userID, phoneNumber string) error {
	_, err := a.db.Exec(a.queries.deletePhoneNumber, userID, phoneNumber)
	return err
}

func (a *Manager) readPhoneNumber(rows *sql.Rows) (string, error) {
	var phoneNumber string
	if !rows.Next() {
		return "", ErrPhoneNumberNotFound
	}
	if err := rows.Scan(&phoneNumber); err != nil {
		return "", err
	} else if err := rows.Err(); err != nil {
		return "", err
	}
	return phoneNumber, nil
}

// Emails returns all verified email addresses for the user with the given user ID
func (a *Manager) Emails(userID string) ([]string, error) {
	rows, err := a.db.ReadOnly().Query(a.queries.selectEmails, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	emails := make([]string, 0)
	for {
		email, err := a.readEmail(rows)
		if errors.Is(err, ErrEmailNotFound) {
			break
		} else if err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, nil
}

// AddEmail adds a verified email address to the user with the given user ID
func (a *Manager) AddEmail(userID, email string) error {
	if _, err := a.db.Exec(a.queries.insertEmail, userID, email); err != nil {
		if isUniqueConstraintError(err) {
			return ErrEmailExists
		}
		return err
	}
	return nil
}

// RemoveEmail deletes a verified email address from the user with the given user ID
func (a *Manager) RemoveEmail(userID, email string) error {
	_, err := a.db.Exec(a.queries.deleteEmail, userID, email)
	return err
}

func (a *Manager) readEmail(rows *sql.Rows) (string, error) {
	var email string
	if !rows.Next() {
		return "", ErrEmailNotFound
	}
	if err := rows.Scan(&email); err != nil {
		return "", err
	} else if err := rows.Err(); err != nil {
		return "", err
	}
	return email, nil
}

// ChangeBilling updates a user's billing fields
func (a *Manager) ChangeBilling(username string, billing *Billing) error {
	if _, err := a.db.Exec(a.queries.updateBilling, nullString(billing.StripeCustomerID), nullString(billing.StripeSubscriptionID), nullString(string(billing.StripeSubscriptionStatus)), nullString(string(billing.StripeSubscriptionInterval)), nullInt64(billing.StripeSubscriptionPaidUntil.Unix()), nullInt64(billing.StripeSubscriptionCancelAt.Unix()), username); err != nil {
		return err
	}
	return nil
}

// maybeProvisionUsersAccessAndTokens provisions users, access control entries, and tokens based on the config.
func (a *Manager) maybeProvisionUsersAccessAndTokens() error {
	if !a.config.ProvisionEnabled {
		return nil
	}
	// If there is nothing to provision, remove any previously provisioned items using
	// cheap targeted queries, avoiding the expensive Users() call that loads all users.
	if len(a.config.Users) == 0 && len(a.config.Access) == 0 && len(a.config.Tokens) == 0 {
		return a.removeAllProvisioned()
	}
	// If there are provisioned users, do it the slow way
	existingUsers, err := a.Users()
	if err != nil {
		return err
	}
	provisionUsernames := util.Map(a.config.Users, func(u *User) string {
		return u.Name
	})
	existingTokens, err := a.allProvisionedTokens()
	if err != nil {
		return err
	}
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		if err := a.maybeProvisionUsers(tx, provisionUsernames, existingUsers); err != nil {
			return fmt.Errorf("failed to provision users: %v", err)
		}
		if err := a.maybeProvisionGrants(tx); err != nil {
			return fmt.Errorf("failed to provision grants: %v", err)
		}
		if err := a.maybeProvisionTokens(tx, provisionUsernames, existingTokens); err != nil {
			return fmt.Errorf("failed to provision tokens: %v", err)
		}
		return nil
	})
}

// removeAllProvisioned removes all provisioned users, access entries, and tokens. This is the fast path
// for when there is nothing to provision, avoiding the expensive Users() call.
func (a *Manager) removeAllProvisioned() error {
	return db.ExecTx(a.db, func(tx *sql.Tx) error {
		if _, err := tx.Exec(a.queries.deleteUserAccessProvisioned); err != nil {
			return err
		}
		if _, err := tx.Exec(a.queries.deleteAllProvisionedTokens); err != nil {
			return err
		}
		if _, err := tx.Exec(a.queries.deleteUsersProvisioned); err != nil {
			return err
		}
		return nil
	})
}

// maybeProvisionUsers checks if the users in the config are provisioned, and adds or updates them.
// It also removes users that are provisioned, but not in the config anymore.
func (a *Manager) maybeProvisionUsers(tx *sql.Tx, provisionUsernames []string, existingUsers []*User) error {
	// Remove users that are provisioned, but not in the config anymore
	for _, user := range existingUsers {
		if user.Name == Everyone {
			continue
		} else if user.Provisioned && !util.Contains(provisionUsernames, user.Name) {
			if err := a.removeUserTx(tx, user.Name); err != nil {
				return fmt.Errorf("failed to remove provisioned user %s: %v", user.Name, err)
			}
		}
	}
	// Add or update provisioned users
	for _, user := range a.config.Users {
		if user.Name == Everyone {
			continue
		}
		existingUser, exists := util.Find(existingUsers, func(u *User) bool {
			return u.Name == user.Name
		})
		if !exists {
			if err := a.addUserTx(tx, user.Name, user.Hash, user.Role, true); err != nil && !errors.Is(err, ErrUserExists) {
				return fmt.Errorf("failed to add provisioned user %s: %v", user.Name, err)
			}
		} else {
			if !existingUser.Provisioned {
				if err := a.changeProvisionedTx(tx, user.Name, true); err != nil {
					return fmt.Errorf("failed to change provisioned status for user %s: %v", user.Name, err)
				}
			}
			if existingUser.Hash != user.Hash {
				if err := a.changePasswordHashTx(tx, user.Name, user.Hash); err != nil {
					return fmt.Errorf("failed to change password for provisioned user %s: %v", user.Name, err)
				}
			}
			if existingUser.Role != user.Role {
				if err := a.changeRoleTx(tx, user.Name, user.Role); err != nil {
					return fmt.Errorf("failed to change role for provisioned user %s: %v", user.Name, err)
				}
			}
		}
	}
	return nil
}

// maybeProvisionGrants removes all provisioned grants, and (re-)adds the grants from the config.
//
// Unlike users and tokens, grants can be just re-added, because they do not carry any state (such as last
// access time) or do not have dependent resources (such as grants or tokens).
func (a *Manager) maybeProvisionGrants(tx *sql.Tx) error {
	// Remove all provisioned grants
	if _, err := tx.Exec(a.queries.deleteUserAccessProvisioned); err != nil {
		return err
	}
	// (Re-)add provisioned grants
	for username, grants := range a.config.Access {
		user, exists := util.Find(a.config.Users, func(u *User) bool {
			return u.Name == username
		})
		if !exists && username != Everyone {
			return fmt.Errorf("user %s is not a provisioned user, refusing to add ACL entry", username)
		} else if user != nil && user.Role == RoleAdmin {
			return fmt.Errorf("adding access control entries is not allowed for admin roles for user %s", username)
		}
		for _, grant := range grants {
			if err := a.resetAccessTx(tx, username, grant.TopicPattern); err != nil {
				return fmt.Errorf("failed to reset access for user %s and topic %s: %v", username, grant.TopicPattern, err)
			}
			if err := a.allowAccessTx(tx, username, grant.TopicPattern, grant.Permission, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Manager) maybeProvisionTokens(tx *sql.Tx, provisionUsernames []string, existingTokens []*Token) error {
	// Remove tokens that are provisioned, but not in the config anymore
	var provisionTokens []string
	for _, userTokens := range a.config.Tokens {
		for _, token := range userTokens {
			provisionTokens = append(provisionTokens, token.Value)
		}
	}
	for _, existingToken := range existingTokens {
		if !slices.Contains(provisionTokens, existingToken.Value) {
			if _, err := tx.Exec(a.queries.deleteProvisionedToken, existingToken.Value); err != nil {
				return fmt.Errorf("failed to remove provisioned token %s: %v", existingToken.Value, err)
			}
		}
	}
	// (Re-)add provisioned tokens
	for username, tokens := range a.config.Tokens {
		if !slices.Contains(provisionUsernames, username) && username != Everyone {
			return fmt.Errorf("user %s is not a provisioned user, refusing to add tokens", username)
		}
		var userID string
		if err := tx.QueryRow(a.queries.selectUserIDFromUsername, username).Scan(&userID); err != nil {
			return fmt.Errorf("failed to find provisioned user %s for provisioned tokens: %v", username, err)
		}
		for _, token := range tokens {
			if _, err := a.createTokenTx(tx, userID, token.Value, token.Label, time.Unix(0, 0), netip.IPv4Unspecified(), time.Unix(0, 0), 0, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// Close closes the underlying database
func (a *Manager) Close() error {
	return a.db.Close()
}

// isUniqueConstraintError checks if the error is a unique constraint violation for both SQLite and PostgreSQL
func isUniqueConstraintError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "UNIQUE constraint failed") || strings.Contains(errStr, "23505")
}
