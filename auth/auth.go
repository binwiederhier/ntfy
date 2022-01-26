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
	Users() ([]*User, error)
	User(username string) (*User, error)
	ChangePassword(username, password string) error
	ChangeRole(username string, role Role) error
	DefaultAccess() (read bool, write bool)
	AllowAccess(username string, topic string, read bool, write bool) error
	ResetAccess(username string, topic string) error
}

type User struct {
	Name   string
	Hash   string // password hash (bcrypt)
	Role   Role
	Grants []Grant
}

type Grant struct {
	Topic string
	Read  bool
	Write bool
}

type Permission int

const (
	PermissionRead  = Permission(1)
	PermissionWrite = Permission(2)
)

type Role string

const (
	RoleAdmin     = Role("admin")
	RoleUser      = Role("user")
	RoleAnonymous = Role("anonymous")
)

const (
	Everyone = "*"
)

func AllowedRole(role Role) bool {
	return role == RoleUser || role == RoleAdmin
}

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
)
