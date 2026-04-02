//go:build nofirebase

package server

import (
	"errors"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
)

const (
	// FirebaseAvailable is a constant used to indicate that Firebase support is available.
	// It can be disabled with the 'nofirebase' build tag.
	FirebaseAvailable = false
)

var (
	errFirebaseNotAvailable      = errors.New("Firebase not available")
	errFirebaseTemporarilyBanned = errors.New("visitor temporarily banned from using Firebase")
)

type firebaseClient struct {
}

func (c *firebaseClient) Send(v *visitor, m *model.Message) error {
	return errFirebaseNotAvailable
}

type firebaseSender interface {
	Send(m string) error
}

func newFirebaseClient(sender firebaseSender, auther user.Auther) *firebaseClient {
	return nil
}

func newFirebaseSender(credentialsFile string) (firebaseSender, error) {
	return nil, errFirebaseNotAvailable
}
