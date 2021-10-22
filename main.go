package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type Message struct {
	Time int64 `json:"time"`
	Message string `json:"message"`
}

type Channel struct {
	id string
	listeners map[int]listener
	last time.Time
	ctx context.Context
	mu sync.Mutex
}

type Server struct {
	channels map[string]*Channel
	mu sync.Mutex
}

type listener func(msg *Message)

func main() {
	s := &Server{
		channels: make(map[string]*Channel),
	}
	go func() {
		for {
			time.Sleep(5 * time.Second)
			s.mu.Lock()
			log.Printf("channels: %d", len(s.channels))
			s.mu.Unlock()
		}
	}()
	http.HandleFunc("/", s.handle)
	if err := http.ListenAndServe(":9997", nil); err != nil {
		log.Fatalln(err)
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if err := s.handleInternal(w, r); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, err.Error())
	}
}

func (s *Server) handleInternal(w http.ResponseWriter, r *http.Request) error {
	if len(r.URL.Path) == 0 {
		return errors.New("invalid path")
	}
	channel := s.channel(r.URL.Path[1:])
	switch r.Method {
	case http.MethodGet:
		return s.handleGET(w, r, channel)
	case http.MethodPut:
		return s.handlePUT(w, r, channel)
	default:
		return errors.New("invalid method")
	}
}

func (s *Server) handleGET(w http.ResponseWriter, r *http.Request, ch *Channel) error {
	fl, ok := w.(http.Flusher)
	if !ok {
		return errors.New("not a flusher")
	}
	listenerID := rand.Int()
	l := func (msg *Message) {
		json.NewEncoder(w).Encode(&msg)
		fl.Flush()
	}
	ch.mu.Lock()
	ch.listeners[listenerID] = l
	ch.last = time.Now()
	ch.mu.Unlock()
	select {
	case <-ch.ctx.Done():
	case <-r.Context().Done():
	}
	ch.mu.Lock()
	delete(ch.listeners, listenerID)
	if len(ch.listeners) == 0 {
		s.mu.Lock()
		delete(s.channels, ch.id)
		s.mu.Unlock()
	}
	ch.mu.Unlock()
	return nil
}

func (s *Server) handlePUT(w http.ResponseWriter, r *http.Request, ch *Channel) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if len(ch.listeners) == 0 {
		return errors.New("no listeners")
	}
	defer r.Body.Close()
	ch.last = time.Now()
	msg, _ := io.ReadAll(r.Body)
	for _, l := range ch.listeners {
		l(&Message{
			Time:    time.Now().UnixMilli(),
			Message: string(msg),
		})
	}
	return nil
}

func (s *Server) channel(channelID string) *Channel {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.channels[channelID]
	if !ok {
		ctx, _ := context.WithCancel(context.Background()) // FIXME
		c = &Channel{
			id:        channelID,
			listeners: make(map[int]listener),
			last:      time.Now(),
			ctx:       ctx,
			mu:        sync.Mutex{},
		}
		s.channels[channelID] = c
	}
	return c
}
