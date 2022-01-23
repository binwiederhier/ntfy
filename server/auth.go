package server

// auth is a generic interface to implement password-based authentication and authorization
type auth interface {
	Authenticate(user, pass string) (*user, error)
	Authorize(user *user, topic string, perm int) error
}

type user struct {
	Name string
	Role string
}

const (
	permRead  = 1
	permWrite = 2
)

const (
	roleAdmin = "admin"
	roleUser  = "user"
	roleNone  = "none"
)

var everyone = &user{
	Name: "",
	Role: roleNone,
}
