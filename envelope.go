package melody

type envelope struct {
	t      int
	msg    []byte
	filter filterFunc
	// extends
	c string
}
