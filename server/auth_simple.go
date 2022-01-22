package server

/*
sqlite> create table user (id int auto increment, user text, password text not null);
sqlite> create table user_topic (user_id int not null, topic text not null, allow_write int, allow_read int);
sqlite> create table topic (topic text primary key, allow_anonymous_write int, allow_anonymous_read int);
*/

const (
	permRead  = 1
	permWrite = 2
)

type auther interface {
	Authenticate(user, pass string) bool
	Authorize(user, topic string, perm int) bool
}

type memAuther struct {
}

func (m memAuther) Authenticate(user, pass string) bool {
	return user == "phil" && pass == "phil"
}

func (m memAuther) Authorize(user, topic string, perm int) bool {
	if perm == permRead {
		return true
	}
	return user == "phil" && topic == "mytopic"
}
