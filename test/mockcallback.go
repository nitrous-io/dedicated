package test

type MockCallback struct {
	InvokeCalled bool
	To           string
	Payload      string
}

func (self *MockCallback) Invoke(to string, payload string) {
	self.InvokeCalled = true
	self.To = to
	self.Payload = payload
}
