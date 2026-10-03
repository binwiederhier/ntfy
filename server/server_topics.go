package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
)

// handleTopicsGet implements GET /v1/topics?visibility=shared. It returns every reserved topic
// that its owner has marked as shared, together with the owner's username. It is readable by any
// authenticated user (admins and regular users alike); anonymous visitors are rejected earlier
// by ensureUser.
func (s *Server) handleTopicsGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	visibility := strings.ToLower(readQueryParam(r, "visibility"))
	if visibility != string(user.VisibilityShared) {
		return errHTTPBadRequest // Only ?visibility=shared is defined for the prototype
	}
	logvr(v, r).Tag(tagAccount).Debug("Listing shared topics")
	shared, err := s.userManager.SharedTopics()
	if err != nil {
		return err
	}
	response := &apiTopicsResponse{Topics: make([]*apiTopicEntry, 0)}
	for _, t := range shared {
		response.Topics = append(response.Topics, &apiTopicEntry{Topic: t.Topic, Owner: t.Owner})
	}
	return s.writeJSON(w, response)
}

// handleTopicVisibilityPatch implements PATCH /v1/topics/<topic>. It lets the reservation's owner
// (or an admin) toggle a topic between private and shared. Switching to shared announces the topic
// on the directory channel; switching back to private does not retract the announcement.
func (s *Server) handleTopicVisibilityPatch(w http.ResponseWriter, r *http.Request, v *visitor) error {
	matches := apiTopicsSingleRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidPath
	}
	topic := matches[1]
	if !topicRegex.MatchString(topic) {
		return errHTTPBadRequestTopicInvalid
	}
	req, err := readJSONWithLimit[apiTopicVisibilityPatchRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return err
	}
	visibility, err := user.ParseVisibility(req.Visibility)
	if err != nil {
		return errHTTPBadRequest
	}
	u := v.User()
	_, ownerUserID, err := s.userManager.TopicVisibility(topic)
	if err != nil {
		return err
	} else if ownerUserID == "" {
		return errHTTPNotFound // Topic is not reserved, so there is no owner to change
	}
	if ownerUserID != u.ID && !u.IsAdmin() {
		return errHTTPForbidden // Only the owner or an admin may change visibility
	}
	logvr(v, r).
		Tag(tagAccount).
		Fields(log.Context{
			"topic":      topic,
			"owner":      ownerUserID,
			"visibility": string(visibility),
		}).
		Info("Changing topic visibility to %s", visibility)
	if err := s.userManager.SetTopicVisibility(ownerUserID, topic, visibility); err != nil {
		return err
	}
	if visibility == user.VisibilityShared {
		t, err := s.userManager.UserByID(ownerUserID)
		if err != nil {
			return err
		}
		if err := s.publishSharedTopicEvent(v, topic, t.Name); err != nil {
			logvr(v, r).Tag(tagAccount).Err(err).Warn("Failed to announce shared topic")
		}
	}
	return s.writeJSON(w, &apiTopicVisibilityResponse{
		Topic:      topic,
		Visibility: string(visibility),
	})
}

// publishSharedTopicEvent announces a newly shared topic on the well-known directory topic, to
// which authenticated clients subscribe. It is best-effort: a failure to publish must not fail
// the request that changed the visibility.
func (s *Server) publishSharedTopicEvent(v *visitor, topic, owner string) error {
	t, err := s.topicFromID(nil, directoryTopicID) // Internal: no rate limit
	if err != nil {
		return err
	}
	m := model.NewDefaultMessage(t.ID, fmt.Sprintf("topic `%s` was shared by `%s`", topic, owner))
	m.Expires = time.Unix(m.Time, 0).Add(s.config.CacheDuration).Unix()
	// Cache the announcement so clients that are not currently subscribed can still poll it.
	if err := s.messageCache.AddMessage(m); err != nil {
		return err
	}
	go s.pruneMessages()
	return s.dispatch(v, t, m, dispatchOpts{})
}
