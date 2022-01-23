package auth

import "errors"

// Auther is a generic interface to implement password-based authentication and authorization
type Auther interface {
	Authenticate(user, pass string) (*User, error)
	Authorize(user *User, topic string, perm Permission) error
}

type Manager interface {
	AddUser(username, password string, role Role) error
	RemoveUser(username string) error
	ChangePassword(username, password string) error
}

type User struct {
	Name string
	Role Role
}

type Permission int

const (
	PermissionRead  = Permission(1)
	PermissionWrite = Permission(2)
)

type Role string

const (
	RoleAdmin = Role("admin")
	RoleUser  = Role("user")
	RoleNone  = Role("none")
)

var Everyone = &User{
	Name: "",
	Role: RoleNone,
}

var ErrUnauthorized = errors.New("unauthorized")
