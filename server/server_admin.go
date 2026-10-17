package server

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heckel.io/ntfy/v2/user"
)

const (
	// tierReset removes a user's tier when passed as "tier" (same convention as "ntfy user change-tier")
	tierReset = "-"

	// headerTotalCount carries the total number of matching rows of a paginated list response
	headerTotalCount = "X-Total-Count"
)

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request, v *visitor) error {
	return s.writeJSON(w, &apiVersionResponse{
		Version: s.config.BuildVersion,
		Commit:  s.config.BuildCommit,
		Date:    s.config.BuildDate,
	})
}

// handleUsersGet lists users. Without query parameters, all users are returned (as it always was).
// With "limit", "offset" and/or "search", a page of users is returned; the total number of matching
// users is passed in the X-Total-Count header.
func (s *Server) handleUsersGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	search, limit, offset, err := readPaginationParams(r)
	if err != nil {
		return err
	}
	page, err := s.userManager.UsersPage(&user.UserFilter{
		Search: search,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return err
	}
	usersResponse := make([]*apiUserResponse, len(page.Users))
	for i, u := range page.Users {
		tier := ""
		if u.Tier != nil {
			tier = u.Tier.Code
		}
		grants := page.Grants[u.ID]
		userGrants := make([]*apiUserGrantResponse, len(grants))
		for j, g := range grants {
			userGrants[j] = &apiUserGrantResponse{
				Topic:       g.TopicPattern,
				Permission:  g.Permission.String(),
				Provisioned: g.Provisioned,
			}
		}
		reservations := page.Reservations[u.ID]
		userReservations := make([]*apiUserReservationResponse, len(reservations))
		for j, res := range reservations {
			userReservations[j] = &apiUserReservationResponse{
				Topic:    res.Topic,
				Everyone: res.Everyone.String(),
			}
		}
		usersResponse[i] = &apiUserResponse{
			Username:      u.Name,
			Role:          string(u.Role),
			Tier:          tier,
			Grants:        userGrants,
			Reservations:  userReservations,
			Provisioned:   u.Provisioned,
			BillingStatus: string(u.Billing.StripeSubscriptionStatus),
		}
	}
	return s.writeJSONWithTotalCount(w, usersResponse, page.Total)
}

func (s *Server) handleUsersAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUserAddOrUpdateRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !user.AllowedUsername(req.Username) || (req.Password == "" && req.Hash == "") {
		return errHTTPBadRequest.Wrap("username invalid, or password/password_hash missing")
	}
	u, err := s.userManager.User(req.Username)
	if err != nil && !errors.Is(err, user.ErrUserNotFound) {
		return err
	} else if u != nil {
		return errHTTPConflictUserExists
	}
	var tier *user.Tier
	if req.Tier != "" && req.Tier != tierReset {
		tier, err = s.userManager.Tier(req.Tier)
		if errors.Is(err, user.ErrTierNotFound) {
			return errHTTPBadRequestTierInvalid
		} else if err != nil {
			return err
		}
	}
	password, hashed := req.Password, false
	if req.Hash != "" {
		password, hashed = req.Hash, true
	}
	if err := s.userManager.AddUser(req.Username, password, user.RoleUser, hashed); err != nil {
		return err
	}
	if tier != nil {
		if err := s.userManager.ChangeTier(req.Username, req.Tier); err != nil {
			return err
		}
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleUsersUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUserAddOrUpdateRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !user.AllowedUsername(req.Username) {
		return errHTTPBadRequest.Wrap("username invalid")
	} else if req.Password == "" && req.Hash == "" && req.Tier == "" {
		return errHTTPBadRequest.Wrap("need to provide at least one of \"password\", \"password_hash\" or \"tier\"")
	}
	u, err := s.userManager.User(req.Username)
	if err != nil && !errors.Is(err, user.ErrUserNotFound) {
		return err
	} else if u != nil {
		if u.IsAdmin() {
			return errHTTPForbidden
		}
		if req.Hash != "" {
			if err := s.userManager.ChangePassword(req.Username, req.Hash, true); err != nil {
				return mapAdminUserError(err)
			}
		} else if req.Password != "" {
			if err := s.userManager.ChangePassword(req.Username, req.Password, false); err != nil {
				return mapAdminUserError(err)
			}
		}
	} else {
		password, hashed := req.Password, false
		if req.Hash != "" {
			password, hashed = req.Hash, true
		}
		if err := s.userManager.AddUser(req.Username, password, user.RoleUser, hashed); err != nil {
			return err
		}
	}
	if req.Tier == tierReset {
		if err := s.userManager.ResetTier(req.Username); err != nil {
			return mapAdminUserError(err)
		}
	} else if req.Tier != "" {
		if _, err = s.userManager.Tier(req.Tier); errors.Is(err, user.ErrTierNotFound) {
			return errHTTPBadRequestTierInvalid
		} else if err != nil {
			return err
		}
		if err := s.userManager.ChangeTier(req.Username, req.Tier); err != nil {
			return mapAdminUserError(err)
		}
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleUsersDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiUserDeleteRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(req.Username)
	if errors.Is(err, user.ErrUserNotFound) {
		return errHTTPBadRequestUserNotFound
	} else if err != nil {
		return err
	} else if !u.IsUser() {
		return errHTTPUnauthorized.Wrap("can only remove regular users from API")
	} else if u.Provisioned {
		return errHTTPConflictProvisionedUserChange
	}
	// Clean up external state first (like account deletion does), so a paying user is not left with a
	// live Stripe subscription that is no longer linked to any account
	if s.webPush != nil {
		if err := s.webPush.RemoveSubscriptionsByUserID(u.ID); err != nil {
			logvr(v, r).Tag(tagAdmin).Err(err).Warn("Error removing web push subscriptions for %s", u.Name)
		}
	}
	if u.Billing.StripeSubscriptionID != "" && s.stripe != nil {
		logvr(v, r).Tag(tagAdmin).Info("Canceling billing subscription for user %s", u.Name)
		if _, err := s.stripe.CancelSubscription(u.Billing.StripeSubscriptionID); err != nil {
			return err
		}
	}
	if err := s.maybeRemoveMessagesAndExcessReservations(r, v, u, 0); err != nil {
		return err
	}
	logvr(v, r).Tag(tagAdmin).Info("Removing user %s", u.Name)
	if err := s.userManager.RemoveUser(req.Username); err != nil {
		return mapAdminUserError(err)
	}
	if err := s.killUserSubscriber(u, "*"); err != nil { // FIXME super inefficient
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// handleAccessGet lists the entire access control list (including Everyone entries and the entries
// created by topic reservations). Pagination works like in handleUsersGet; "search" matches the
// topic or the username.
func (s *Server) handleAccessGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	search, limit, offset, err := readPaginationParams(r)
	if err != nil {
		return err
	}
	entries, total, err := s.userManager.AccessEntries(&user.AccessFilter{
		Search: search,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return err
	}
	response := make([]*apiAccessEntryResponse, len(entries))
	for i, e := range entries {
		response[i] = &apiAccessEntryResponse{
			Username:    e.Username,
			Topic:       e.TopicPattern,
			Permission:  e.Permission.String(),
			Owner:       e.Owner,
			Provisioned: e.Provisioned,
		}
	}
	return s.writeJSONWithTotalCount(w, response, total)
}

func (s *Server) handleAccessAllow(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccessAllowRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(req.Username)
	if errors.Is(err, user.ErrUserNotFound) {
		return errHTTPBadRequestUserNotFound
	} else if err != nil {
		return err
	} else if u.IsAdmin() {
		return errHTTPBadRequestAccessAdminUser
	}
	permission, err := user.ParsePermission(req.Permission)
	if err != nil {
		return errHTTPBadRequestPermissionInvalid
	}
	if err := s.checkAccessChangeAllowed(u, req.Topic); err != nil {
		return err
	}
	if err := s.userManager.AllowAccess(req.Username, req.Topic, permission); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAccessReset(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAccessResetRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	u, err := s.userManager.User(req.Username)
	if errors.Is(err, user.ErrUserNotFound) {
		return errHTTPBadRequestUserNotFound
	} else if err != nil {
		return err
	}
	if err := s.checkAccessResetAllowed(u, req.Topic); err != nil {
		return err
	}
	if err := s.userManager.ResetAccess(req.Username, req.Topic); err != nil {
		return err
	}
	if err := s.killUserSubscriber(u, req.Topic); err != nil { // This may be a pattern
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// checkAccessChangeAllowed rejects overwriting a provisioned entry (it would come back on restart),
// and overwriting an entry of a topic reservation (the reservation would silently turn into a plain
// grant). The owner's entry and the Everyone entry of a reservation belong to the reservation.
func (s *Server) checkAccessChangeAllowed(u *user.User, topicPattern string) error {
	if err := s.checkAccessResetAllowed(u, topicPattern); err != nil {
		return err
	}
	ownerID, err := s.userManager.ReservationOwner(topicPattern)
	if err != nil {
		return err
	} else if ownerID != "" && ownerID == u.ID {
		return errHTTPConflictReservationAccessChange
	}
	return nil
}

// checkAccessResetAllowed rejects removing provisioned entries (if topicPattern is empty, all of
// the user's entries are removed), and removing only the Everyone entry of a reservation. Removing
// the owner's entry removes the entire reservation, which is allowed.
func (s *Server) checkAccessResetAllowed(u *user.User, topicPattern string) error {
	grants, err := s.userManager.Grants(u.Name)
	if err != nil {
		return err
	}
	for _, g := range grants {
		if g.Provisioned && (topicPattern == "" || g.TopicPattern == topicPattern) {
			return errHTTPConflictProvisionedAccessChange
		}
	}
	if u.Name == user.Everyone && topicPattern != "" {
		ownerID, err := s.userManager.ReservationOwner(topicPattern)
		if err != nil {
			return err
		} else if ownerID != "" {
			return errHTTPConflictReservationAccessChange
		}
	}
	return nil
}

// handleReservationsGet lists the topic reservations of all users. Pagination works like in
// handleUsersGet; "search" matches the topic or the username of the owner.
func (s *Server) handleReservationsGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	search, limit, offset, err := readPaginationParams(r)
	if err != nil {
		return err
	}
	entries, total, err := s.userManager.ReservationEntries(&user.AccessFilter{
		Search: search,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return err
	}
	response := make([]*apiReservationEntryResponse, len(entries))
	for i, e := range entries {
		response[i] = &apiReservationEntryResponse{
			Username: e.Username,
			Topic:    e.Topic,
			Everyone: e.Everyone.String(),
		}
	}
	return s.writeJSONWithTotalCount(w, response, total)
}

// handleReservationsUpdate reserves a topic for a user, or changes the access for everyone else
// of an existing reservation. Unlike reservations made by users themselves, the tier's
// reservation limit does not apply.
func (s *Server) handleReservationsUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiReservationRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !user.AllowedTopic(req.Topic) {
		return errHTTPBadRequestTopicInvalid
	}
	everyone, err := user.ParsePermission(req.Everyone)
	if err != nil {
		return errHTTPBadRequestPermissionInvalid
	}
	u, err := s.readReservationOwner(req.Username)
	if err != nil {
		return err
	}
	if err := s.userManager.AllowReservation(u.Name, req.Topic); err != nil {
		return errHTTPConflictTopicReserved
	}
	logvr(v, r).Tag(tagAdmin).Info("Reserving topic %s for user %s, everyone access %s", req.Topic, u.Name, everyone.String())
	if err := s.userManager.AddReservation(u.Name, req.Topic, everyone, 0); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleReservationsDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiReservationRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	} else if !user.AllowedTopic(req.Topic) {
		return errHTTPBadRequestTopicInvalid
	}
	u, err := s.readReservationOwner(req.Username)
	if err != nil {
		return err
	}
	if hasReservation, err := s.userManager.HasReservation(u.Name, req.Topic); err != nil {
		return err
	} else if !hasReservation {
		return errHTTPBadRequestNoTopicReservation
	}
	logvr(v, r).Tag(tagAdmin).Info("Removing reservation of topic %s for user %s", req.Topic, u.Name)
	if err := s.userManager.RemoveReservations(u.Name, req.Topic); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

// readReservationOwner returns the user with the given name, if it is a regular user (only regular
// users can own topic reservations)
func (s *Server) readReservationOwner(username string) (*user.User, error) {
	u, err := s.userManager.User(username)
	if errors.Is(err, user.ErrUserNotFound) {
		return nil, errHTTPBadRequestUserNotFound
	} else if err != nil {
		return nil, err
	} else if !u.IsUser() {
		return nil, errHTTPBadRequest.Wrap("only regular users can own topic reservations")
	}
	return u, nil
}

func (s *Server) handleAdminTiersGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	tiers, err := s.userManager.Tiers()
	if err != nil {
		return err
	}
	counts, err := s.userManager.TierUsersCount()
	if err != nil {
		return err
	}
	slices.SortFunc(tiers, func(a, b *user.Tier) int {
		return strings.Compare(a.Code, b.Code)
	})
	response := make([]*apiAdminTier, len(tiers))
	for i, t := range tiers {
		response[i] = &apiAdminTier{
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
			Users:                    counts[t.Code],
		}
	}
	return s.writeJSON(w, response)
}

func (s *Server) handleAdminTiersAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	tier, err := readAdminTierRequest(r)
	if err != nil {
		return err
	}
	if _, err := s.userManager.Tier(tier.Code); err == nil {
		return errHTTPConflictTierExists
	} else if !errors.Is(err, user.ErrTierNotFound) {
		return err
	}
	logvr(v, r).Tag(tagAdmin).Info("Adding tier %s", tier.Code)
	if err := s.userManager.AddTier(tier); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAdminTiersUpdate(w http.ResponseWriter, r *http.Request, v *visitor) error {
	tier, err := readAdminTierRequest(r)
	if err != nil {
		return err
	}
	if _, err := s.userManager.Tier(tier.Code); errors.Is(err, user.ErrTierNotFound) {
		return errHTTPBadRequestTierInvalid
	} else if err != nil {
		return err
	}
	logvr(v, r).Tag(tagAdmin).Info("Updating tier %s", tier.Code)
	if err := s.userManager.UpdateTier(tier); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) handleAdminTiersDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	req, err := readJSONWithLimit[apiAdminTierDeleteRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	if _, err := s.userManager.Tier(req.Code); errors.Is(err, user.ErrTierNotFound) {
		return errHTTPBadRequestTierInvalid
	} else if err != nil {
		return err
	}
	counts, err := s.userManager.TierUsersCount()
	if err != nil {
		return err
	} else if counts[req.Code] > 0 {
		return errHTTPConflictTierInUse
	}
	logvr(v, r).Tag(tagAdmin).Info("Removing tier %s", req.Code)
	if err := s.userManager.RemoveTier(req.Code); err != nil {
		return err
	}
	return s.writeJSON(w, newSuccessResponse())
}

func (s *Server) killUserSubscriber(u *user.User, topicPattern string) error {
	topics, err := s.topicsFromPattern(topicPattern)
	if err != nil {
		return err
	}
	for _, t := range topics {
		t.CancelSubscriberUser(u.ID)
	}
	return nil
}

// writeJSONWithTotalCount writes a (page of a) list, passing the total number of rows in a header,
// so that the response body stays a plain JSON array (as it was before pagination existed)
func (s *Server) writeJSONWithTotalCount(w http.ResponseWriter, v any, total int64) error {
	w.Header().Set(headerTotalCount, strconv.FormatInt(total, 10))
	w.Header().Set("Access-Control-Expose-Headers", headerTotalCount) // CORS, allow JS to read the header
	return s.writeJSON(w, v)
}

// readPaginationParams reads the optional "search", "limit" and "offset" query parameters.
// A zero limit means no limit.
func readPaginationParams(r *http.Request) (search string, limit, offset int, err error) {
	query := r.URL.Query()
	search = strings.TrimSpace(query.Get("search"))
	if limit, err = readNonNegativeIntParam(query.Get("limit")); err != nil {
		return "", 0, 0, err
	}
	if offset, err = readNonNegativeIntParam(query.Get("offset")); err != nil {
		return "", 0, 0, err
	}
	return search, limit, offset, nil
}

func readNonNegativeIntParam(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	i, err := strconv.Atoi(value)
	if err != nil || i < 0 {
		return 0, errHTTPBadRequestPaginationInvalid
	}
	return i, nil
}

// readAdminTierRequest reads and validates an apiAdminTier request, and converts it to a user.Tier
func readAdminTierRequest(r *http.Request) (*user.Tier, error) {
	req, err := readJSONWithLimit[apiAdminTier](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return nil, err
	} else if !user.AllowedTier(req.Code) || strings.TrimSpace(req.Name) == "" {
		return nil, errHTTPBadRequestTierCodeInvalid
	}
	return &user.Tier{
		Code:                     req.Code,
		Name:                     strings.TrimSpace(req.Name),
		MessageLimit:             req.MessagesLimit,
		MessageExpiryDuration:    time.Duration(req.MessagesExpiryDuration) * time.Second,
		EmailLimit:               req.EmailsLimit,
		CallLimit:                req.CallsLimit,
		ReservationLimit:         req.ReservationsLimit,
		AttachmentFileSizeLimit:  req.AttachmentFileSizeLimit,
		AttachmentTotalSizeLimit: req.AttachmentTotalSizeLimit,
		AttachmentExpiryDuration: time.Duration(req.AttachmentExpiryDuration) * time.Second,
		AttachmentBandwidthLimit: req.AttachmentBandwidthLimit,
		StripeMonthlyPriceID:     req.StripeMonthlyPriceID,
		StripeYearlyPriceID:      req.StripeYearlyPriceID,
	}, nil
}

// mapAdminUserError maps user manager errors of user changes to HTTP errors
func mapAdminUserError(err error) error {
	if errors.Is(err, user.ErrProvisionedUserChange) {
		return errHTTPConflictProvisionedUserChange
	} else if errors.Is(err, user.ErrTooManyReservations) {
		return errHTTPBadRequestTierReservationsLimit
	}
	return err
}
