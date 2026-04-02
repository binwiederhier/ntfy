package server

import (
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/util"
)

func (s *Server) execManager() {
	// WARNING: Make sure to only selectively lock with the mutex, and be aware that this
	//          there is no mutex for the entire function.

	// Prune all the things
	s.pruneVisitors()
	s.pruneTokens()
	s.pruneAttachments()
	s.pruneMessages()
	s.pruneAndNotifyWebPushSubscriptions()

	// Message count
	messagesCached, err := s.messageCache.MessagesCount()
	if err != nil {
		log.Tag(tagManager).Err(err).Warn("Cannot get messages count")
	}

	// Remove subscriptions without subscribers
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
	if s.smtpSender != nil {
		sentMailTotal, sentMailSuccess, sentMailFailure = s.smtpSender.Counts()
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
	mset(metricMessagesCached, messagesCached)
	mset(metricVisitors, visitorsCount)
	mset(metricUsers, usersCount)
	mset(metricSubscribers, subscribers)
	mset(metricTopics, topicsCount)
	if s.attachment != nil {
		mset(metricAttachmentsTotalSize, s.attachment.Size())
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
