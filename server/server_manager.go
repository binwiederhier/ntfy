package server

import (
	"heckel.io/ntfy/log"
	"strings"
)

func (s *Server) execManager() {
	// WARNING: Make sure to only selectively lock with the mutex, and be aware that this
	//          there is no mutex for the entire function.

	// Prune all the things
	s.pruneVisitors()
	s.pruneTokens()
	s.pruneAttachments()
	s.pruneMessages()

	// Message count per topic
	var messagesCached int
	messageCounts, err := s.messageCache.MessageCounts()
	if err != nil {
		log.Tag(tagManager).Err(err).Warn("Cannot get message counts")
		messageCounts = make(map[string]int) // Empty, so we can continue
	}
	for _, count := range messageCounts {
		messagesCached += count
	}

	// Remove subscriptions without subscribers
	var emptyTopics, subscribers int
	log.
		Tag(tagManager).
		Timing(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			for _, t := range s.topics {
				subs := t.SubscribersCount()
				ev := log.Tag(tagManager)
				if ev.IsTrace() {
					vrate := t.RateVisitor()
					if vrate != nil {
						ev.Fields(log.Context{
							"rate_visitor_ip":      vrate.IP(),
							"rate_visitor_user_id": vrate.MaybeUserID(),
						})
					}
					ev.
						Fields(log.Context{
							"message_topic":             t.ID,
							"message_topic_subscribers": subs,
						}).
						Trace("- topic %s: %d subscribers", t.ID, subs)
				}
				msgs, exists := messageCounts[t.ID]
				if t.Stale() && (!exists || msgs == 0) {
					log.Tag(tagManager).Field("message_topic", t.ID).Trace("Deleting empty topic %s", t.ID)
					emptyTopics++
					delete(s.topics, t.ID)
					continue
				}
				subscribers += subs
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

	// Print stats
	s.mu.Lock()
	messagesCount, topicsCount, visitorsCount := s.messages, len(s.topics), len(s.visitors)
	s.mu.Unlock()
	log.
		Tag(tagManager).
		Fields(log.Context{
			"messages_published":      messagesCount,
			"messages_cached":         messagesCached,
			"topics_active":           topicsCount,
			"subscribers":             subscribers,
			"visitors":                visitorsCount,
			"emails_received":         receivedMailTotal,
			"emails_received_success": receivedMailSuccess,
			"emails_received_failure": receivedMailFailure,
			"emails_sent":             sentMailTotal,
			"emails_sent_success":     sentMailSuccess,
			"emails_sent_failure":     sentMailFailure,
		}).
		Info("Server stats")
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
		Debug("Deleted %d stale visitor(s)", staleVisitors)
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
			Debug("Removed expired tokens and users")
	}
}

func (s *Server) pruneAttachments() {
	if s.fileCache == nil {
		return
	}
	log.
		Tag(tagManager).
		Timing(func() {
			ids, err := s.messageCache.AttachmentsExpired()
			if err != nil {
				log.Tag(tagManager).Err(err).Warn("Error retrieving expired attachments")
			} else if len(ids) > 0 {
				if log.Tag(tagManager).IsDebug() {
					log.Tag(tagManager).Debug("Deleting attachments %s", strings.Join(ids, ", "))
				}
				if err := s.fileCache.Remove(ids...); err != nil {
					log.Tag(tagManager).Err(err).Warn("Error deleting attachments")
				}
				if err := s.messageCache.MarkAttachmentsDeleted(ids...); err != nil {
					log.Tag(tagManager).Err(err).Warn("Error marking attachments deleted")
				}
			} else {
				log.Tag(tagManager).Debug("No expired attachments to delete")
			}
		}).
		Debug("Deleted expired attachments")
}

func (s *Server) pruneMessages() {
	log.
		Tag(tagManager).
		Timing(func() {
			expiredMessageIDs, err := s.messageCache.MessagesExpired()
			if err != nil {
				log.Tag(tagManager).Err(err).Warn("Error retrieving expired messages")
			} else if len(expiredMessageIDs) > 0 {
				if s.fileCache != nil {
					if err := s.fileCache.Remove(expiredMessageIDs...); err != nil {
						log.Tag(tagManager).Err(err).Warn("Error deleting attachments for expired messages")
					}
				}
				if err := s.messageCache.DeleteMessages(expiredMessageIDs...); err != nil {
					log.Tag(tagManager).Err(err).Warn("Error marking attachments deleted")
				}
			} else {
				log.Tag(tagManager).Debug("No expired messages to delete")
			}
		}).
		Debug("Pruned messages")
}
