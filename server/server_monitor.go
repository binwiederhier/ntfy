package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/monitor"
	"heckel.io/ntfy/v2/user"
)

const tagMonitor = "monitor"

// Request and response payloads.

type monitorAddRequest struct {
	Key           string `json:"key"`
	Period        int64  `json:"period"`
	Grace         int64  `json:"grace"`
	AlertTopic    string `json:"alert_topic"`
	AlertPriority *int   `json:"alert_priority,omitempty"` // pointer so 0 is distinguishable from unset
}

type monitorResponse struct {
	Key           string `json:"key"`
	Period        int64  `json:"period"`
	Grace         int64  `json:"grace"`
	AlertTopic    string `json:"alert_topic"`
	AlertPriority int    `json:"alert_priority"`
	State         string `json:"state"`
	LastSeenAt    int64  `json:"last_seen_at"`
	CreatedAt     int64  `json:"created_at"`
}

type monitorListResponse struct {
	Monitors []*monitorResponse `json:"monitors"`
}

type heartbeatResponse struct {
	Key        string `json:"key"`
	State      string `json:"state"`
	PrevState  string `json:"prev_state"`
	LastSeenAt int64  `json:"last_seen_at"`
}

type monitorDeleteResponse struct {
	OK bool `json:"ok"`
}

func monitorToResponse(m *monitor.Monitor) *monitorResponse {
	return &monitorResponse{
		Key:           m.Key,
		Period:        m.PeriodSeconds,
		Grace:         m.GraceSeconds,
		AlertTopic:    m.AlertTopic,
		AlertPriority: m.AlertPriority,
		State:         m.State,
		LastSeenAt:    m.LastSeenAt,
		CreatedAt:     m.CreatedAt,
	}
}

// ensureMonitorsEnabled is a middleware that returns an error if the monitor feature is disabled.
func (s *Server) ensureMonitorsEnabled(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.monitorManager == nil {
			return errHTTPBadRequestMonitorsDisabled
		}
		return next(w, r, v)
	}
}

// keyFromPath extracts the trailing path segment (the monitor key) from a URL like
// /v1/monitors/<key> or /v1/heartbeat/<key>.
func keyFromPath(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 || i == len(path)-1 {
		return ""
	}
	return path[i+1:]
}

func (s *Server) handleMonitorAdd(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	req, err := readJSONWithLimit[monitorAddRequest](r.Body, jsonBodyBytesLimit, false)
	if err != nil {
		return errHTTPBadRequestMonitorBodyInvalid
	}
	priority := monitor.DefaultAlertPriority
	if req.AlertPriority != nil {
		priority = *req.AlertPriority
	}
	mon, err := s.monitorManager.AddMonitor(u.ID, req.Key, req.Period, req.Grace, req.AlertTopic, priority)
	if err != nil {
		return mapMonitorError(err)
	}
	return s.writeJSON(w, monitorToResponse(mon))
}

func (s *Server) handleMonitorList(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	mons, err := s.monitorManager.ListMonitorsByUser(u.ID)
	if err != nil {
		return err
	}
	out := make([]*monitorResponse, 0, len(mons))
	for _, m := range mons {
		out = append(out, monitorToResponse(m))
	}
	return s.writeJSON(w, &monitorListResponse{Monitors: out})
}

func (s *Server) handleMonitorGet(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	key := keyFromPath(r.URL.Path)
	if err := monitor.ValidateKey(key); err != nil {
		return errHTTPBadRequestMonitorKeyInvalid
	}
	mon, err := s.monitorManager.GetMonitor(u.ID, key)
	if err != nil {
		return mapMonitorError(err)
	}
	return s.writeJSON(w, monitorToResponse(mon))
}

func (s *Server) handleMonitorDelete(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	key := keyFromPath(r.URL.Path)
	if err := monitor.ValidateKey(key); err != nil {
		return errHTTPBadRequestMonitorKeyInvalid
	}
	if err := s.monitorManager.DeleteMonitor(u.ID, key); err != nil {
		return mapMonitorError(err)
	}
	return s.writeJSON(w, &monitorDeleteResponse{OK: true})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request, v *visitor) error {
	u := v.User()
	key := keyFromPath(r.URL.Path)
	if err := monitor.ValidateKey(key); err != nil {
		return errHTTPBadRequestMonitorKeyInvalid
	}
	mon, prev, err := s.monitorManager.RecordHeartbeat(u.ID, key)
	if err != nil {
		return mapMonitorError(err)
	}
	if prev == monitor.StateDown {
		go s.publishMonitorAlert(mon, true)
	}
	return s.writeJSON(w, &heartbeatResponse{
		Key:        mon.Key,
		State:      mon.State,
		PrevState:  prev,
		LastSeenAt: mon.LastSeenAt,
	})
}

// publishMonitorAlert publishes a DOWN or UP recovery message to mon.AlertTopic, attributed to
// the monitor owner. Mirrors the sendDelayedMessage path: forward to active subscribers, cache
// for offline subscribers, and (if configured) deliver via Firebase and Web Push.
func (s *Server) publishMonitorAlert(mon *monitor.Monitor, recovery bool) {
	owner, err := s.lookupMonitorOwner(mon)
	if err != nil {
		log.Tag(tagMonitor).Err(err).Warn("Monitor %s: unable to look up owner %s; alert dropped", mon.Key, mon.UserID)
		return
	}
	t, err := s.topicFromID(mon.AlertTopic)
	if err != nil {
		log.Tag(tagMonitor).Err(err).Warn("Monitor %s: unable to resolve alert topic %s; alert dropped", mon.Key, mon.AlertTopic)
		return
	}
	v := s.visitor(netip.IPv4Unspecified(), owner)

	m := model.NewDefaultMessage(mon.AlertTopic, monitorMessageBody(mon, recovery))
	m.Title = monitorMessageTitle(mon, recovery)
	m.Priority = mon.AlertPriority
	if recovery {
		m.Tags = []string{"white_check_mark"}
	} else {
		m.Tags = []string{"warning"}
	}
	m.Expires = time.Now().Add(s.config.CacheDuration).Unix()
	if owner != nil {
		m.User = owner.ID
	}
	m.SanitizeUTF8()

	if err := t.Publish(v, m); err != nil {
		log.Tag(tagMonitor).Err(err).Warn("Monitor %s: unable to publish alert", mon.Key)
	}
	if err := s.messageCache.AddMessage(m); err != nil {
		log.Tag(tagMonitor).Err(err).Warn("Monitor %s: unable to cache alert message", mon.Key)
	}
	if s.firebaseClient != nil {
		go s.sendToFirebase(v, m)
	}
	if s.config.WebPushPublicKey != "" {
		go s.publishToWebPushEndpoints(v, m)
	}
}

func (s *Server) lookupMonitorOwner(mon *monitor.Monitor) (*user.User, error) {
	if s.userManager == nil || mon.UserID == "" {
		return nil, errors.New("user manager not configured or monitor has no owner")
	}
	return s.userManager.UserByID(mon.UserID)
}

func monitorMessageTitle(mon *monitor.Monitor, recovery bool) string {
	if recovery {
		return fmt.Sprintf("Monitor %s is UP", mon.Key)
	}
	return fmt.Sprintf("Monitor %s is DOWN", mon.Key)
}

func monitorMessageBody(mon *monitor.Monitor, recovery bool) string {
	if recovery {
		return fmt.Sprintf("Heartbeat received; monitor %s recovered after being down.", mon.Key)
	}
	return fmt.Sprintf("No heartbeat for at least %ds (period=%ds, grace=%ds).", mon.PeriodSeconds+mon.GraceSeconds, mon.PeriodSeconds, mon.GraceSeconds)
}

// runMonitorChecker periodically scans for stale Up monitors, transitions them to Down, and
// publishes DOWN alerts. Up->Down transitions originate here; Down->Up happens in handleHeartbeat.
func (s *Server) runMonitorChecker() {
	for {
		select {
		case <-time.After(s.config.MonitorCheckInterval):
			if err := s.checkMonitors(); err != nil {
				log.Tag(tagMonitor).Err(err).Warn("Monitor check failed")
			}
		case <-s.closeChan:
			return
		}
	}
}

func (s *Server) checkMonitors() error {
	stale, err := s.monitorManager.StaleMonitors()
	if err != nil {
		return err
	}
	for _, mon := range stale {
		if err := s.monitorManager.MarkDown(mon.ID); err != nil {
			log.Tag(tagMonitor).Err(err).Warn("Monitor %s: failed to mark down", mon.Key)
			continue
		}
		s.publishMonitorAlert(mon, false)
	}
	return nil
}

func mapMonitorError(err error) error {
	switch {
	case errors.Is(err, monitor.ErrMonitorNotFound):
		return errHTTPNotFoundMonitor
	case errors.Is(err, monitor.ErrMonitorExists):
		return errHTTPConflictMonitorExists
	case errors.Is(err, monitor.ErrInvalidKey):
		return errHTTPBadRequestMonitorKeyInvalid
	case errors.Is(err, monitor.ErrInvalidPeriod):
		return errHTTPBadRequestMonitorPeriodInvalid
	case errors.Is(err, monitor.ErrInvalidGrace):
		return errHTTPBadRequestMonitorGraceInvalid
	case errors.Is(err, monitor.ErrInvalidAlertTopic):
		return errHTTPBadRequestMonitorAlertTopicInvalid
	case errors.Is(err, monitor.ErrInvalidAlertPriority):
		return errHTTPBadRequestMonitorAlertPriorityInvalid
	}
	return err
}
