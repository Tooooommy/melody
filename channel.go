package melody

type Channel struct {
	name    string
	online  int
	session *Session
}

func (c *Channel) Online() int {
	return c.online
}

func (c *Channel) Name() string {
	return c.name
}
