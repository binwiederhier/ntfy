package server

import (
	"errors"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func (s *Server) execManager() {
	// WARNING: Make sure to only selectively lock with the mutex, and be aware that this
	//          there is no mutex for the entire function.

	// Prune all the things. In-memory state is pruned on every node; jobs touching shared
	// databases (and the web push job, which also sends expiry-warning notifications) run on
	// the cluster leader only. In a single-node setup, IsLeader is always true.
	s.pruneVisitors()
	if s.cluster.IsLeader() {
		s.pruneTokens()
		s.pruneAttachments()
		s.pruneMessages()
		s.pruneAndNotifyWebPushSubscriptions()
		s.pruneVisitorUsage()
	}

	// Message count
	messagesCached, err := s.messageCache.MessagesCount()
	if err != nil {
		log.Tag(tagManager).Err(err).Warn("Cannot get messages count")
	}

	// Re-check that live subscribers may still read what they are subscribed to
	s.reauthorizeSubscribers()

	// Remove subscriptions without subscribers (unless active on another node)
	s.keepSharedActiveTopics()
	var emptyTopics, subscribers int
	log.
		Tag(tagManager).
		Timing(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			for _, t := range s.topics {
				subs, lastAccess := t.Stats()
				ev := log.Tag(tagManager).With(t)
				if t.Stale() {
					if ev.IsTrace() {
						ev.Trace("- topic %s: Deleting stale topic (%d subscribers, accessed %s)", t.ID, subs, util.FormatTime(lastAccess))
					}
					emptyTopics++
					delete(s.topics, t.ID)
				} else {
					if ev.IsTrace() {
						ev.Trace("- topic %s: %d subscribers, accessed %s", t.ID, subs, util.FormatTime(lastAccess))
					}
					subscribers += subs
				}
			}
		}).
		Debug("Removed %d empty topic(s)", emptyTopics)

	// Mail stats
	var receivedMailTotal, receivedMailSuccess, receivedMailFailure int64
	if s.smtpServerBackend != nil {
		receivedMailTotal, receivedMailSuccess, receivedMailFailure = s.smtpServerBackend.Counts()
	}
	var sentMailTotal, sentMailSuccess, sentMailFailure int64
	if s.mailer != nil {
		sentMailTotal, sentMailSuccess, sentMailFailure = s.mailer.NotificationCounts()
	}

	// Users
	var usersCount int64
	if s.userManager != nil {
		usersCount, err = s.userManager.UsersCount()
		if err != nil {
			log.Tag(tagManager).Err(err).Warn("Error counting users")
		}
	}

	// Print stats
	s.mu.RLock()
	messagesCount, topicsCount, visitorsCount := s.messages, len(s.topics), len(s.visitors)
	s.mu.RUnlock()

	// Update stats
	s.updateAndWriteStats(messagesCount)

	// Log stats
	log.
		Tag(tagManager).
		Fields(log.Context{
			"messages_published":      messagesCount,
			"messages_cached":         messagesCached,
			"topics_active":           topicsCount,
			"subscribers":             subscribers,
			"visitors":                visitorsCount,
			"users":                   usersCount,
			"emails_received":         receivedMailTotal,
			"emails_received_success": receivedMailSuccess,
			"emails_received_failure": receivedMailFailure,
			"emails_sent":             sentMailTotal,
			"emails_sent_success":     sentMailSuccess,
			"emails_sent_failure":     sentMailFailure,
		}).
		Info("Server stats")
	metrics.MessagesCached.Set(float64(messagesCached))
	metrics.Visitors.Set(float64(visitorsCount))
	metrics.Users.Set(float64(usersCount))
	metrics.Subscribers.Set(float64(subscribers))
	metrics.Topics.Set(float64(topicsCount))
	if s.attachment != nil {
		metrics.AttachmentsTotalSize.Set(float64(s.attachment.Size()))
	}
}

func (s *Server) pruneVisitors() {
	staleVisitors := 0
	log.
		Tag(tagManager).
		Timing(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			for ip, v := range s.visitors {
				if v.Stale() {
					log.Tag(tagManager).With(v).Trace("Deleting stale visitor")
					delete(s.visitors, ip)
					staleVisitors++
				}
			}
		}).
		Field("stale_visitors", staleVisitors).
		Debug("Finished deleting stale visitors")
}

func (s *Server) pruneTokens() {
	if s.userManager != nil {
		log.
			Tag(tagManager).
			Timing(func() {
				if err := s.userManager.RemoveExpiredTokens(); err != nil {
					log.Tag(tagManager).Err(err).Warn("Error expiring user tokens")
				}
				if err := s.userManager.RemoveDeletedUsers(); err != nil {
					log.Tag(tagManager).Err(err).Warn("Error deleting soft-deleted users")
				}
			}).
			Debug("Finished deleting expired tokens and users")
	}
}

func (s *Server) pruneAttachments() {
	if s.attachment == nil {
		return
	}
	// Only mark as deleted in DB. The actual storage files are cleaned up
	// by the attachment store's sync() loop, which periodically reconciles
	// storage with the database and removes orphaned files.
	log.
		Tag(tagManager).
		Timing(func() {
			count, err := s.messageCache.MarkExpiredAttachmentsDeleted(s.config.ManagerBatchSize)
			if err != nil {
				log.Tag(tagManager).Err(err).Warn("Error marking expired attachments as deleted")
			} else if count > 0 {
				log.Tag(tagManager).Debug("Marked %d expired attachment(s) as deleted", count)
			} else {
				log.Tag(tagManager).Debug("No expired attachments to delete")
			}
		}).
		Debug("Finished marking expired attachments as deleted")
}

func (s *Server) pruneMessages() {
	// Only delete DB rows. Attachment storage files are cleaned up by the
	// attachment store's sync() loop, which periodically reconciles storage
	// with the database and removes orphaned files.
	log.
		Tag(tagManager).
		Timing(func() {
			count, err := s.messageCache.DeleteExpiredMessages(s.config.ManagerBatchSize)
			if err != nil {
				log.Tag(tagManager).Err(err).Warn("Error deleting expired messages")
			} else if count > 0 {
				log.Tag(tagManager).Debug("Deleted %d expired message(s)", count)
			} else {
				log.Tag(tagManager).Debug("No expired messages to delete")
			}
		}).
		Debug("Finished deleting expired messages")
}

// pruneVisitorUsage deletes old per-day visitor usage rows (cluster mode only); the retention
// lives in the quota package. Leader-only, like the other shared-database prunes.
func (s *Server) pruneVisitorUsage() {
	if s.quota == nil {
		return
	}
	if err := s.quota.Prune(); err != nil {
		log.Tag(tagManager).Err(err).Warn("Error pruning visitor usage")
	}
	if err := s.topicStore.Prune(); err != nil {
		log.Tag(tagManager).Err(err).Warn("Error pruning topic state")
	}
}

// reauthorizeSubscribers cancels live subscriptions whose user may no longer read the topic.
// Access revocation reaches open connections two ways, and neither is reliable on its own: the
// node that ran the revocation cancels its own subscribers and asks its peers to do the same,
// but that request is best-effort peer state and a lost one is never retried; and an ACL change
// made elsewhere (another node, the CLI) only lands in this node's cache at its next reload.
// Re-checking every manager tick makes revocation eventually consistent instead of permanent
// access, and also covers a topic turning private and a deleted user.
//
// The check itself is in-memory (the ACL cache); only the user lookups hit the database, once
// per distinct subscribed user per tick. A user that cannot be looked up keeps its
// subscription: a database hiccup must not disconnect everyone.
func (s *Server) reauthorizeSubscribers() {
	if s.userManager == nil {
		return
	}
	s.mu.RLock()
	topics := make([]*topic, 0, len(s.topics))
	for _, t := range s.topics {
		topics = append(topics, t)
	}
	s.mu.RUnlock()
	users := make(map[string]*user.User) // Resolved once per tick; nil means "no longer exists"
	for _, t := range topics {
		for _, userID := range t.SubscriberUserIDs() {
			u, resolved := users[userID]
			if !resolved {
				var err error
				if u, err = s.userManager.UserByID(userID); errors.Is(err, user.ErrUserNotFound) {
					u = nil // Deleted; its subscriptions go below
				} else if err != nil {
					log.Tag(tagManager).Err(err).Warn("Cannot re-authorize subscribers of user %s", userID)
					continue // Leave the subscription alone
				}
				users[userID] = u
			}
			if u == nil || s.userManager.Authorize(u, t.ID, user.PermissionRead) != nil {
				log.Tag(tagManager).With(t).Debug("Canceling subscriber of user %s: no longer authorized to read", userID)
				t.CancelSubscriberUser(userID)
			}
		}
	}
}
