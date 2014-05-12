package freightertest

type MockCallback struct {
	InvokeCalled bool
	To           string
	Payload      string
}

func (c *MockCallback) Invoke(to string, payload string) {
	c.InvokeCalled = true
	c.To = to
	c.Payload = payload
}
