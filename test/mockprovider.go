package freightertest

type MockProvider struct {
	CreateCalled bool
	Name         string
	PublicKeys   []string
	WebHookUrl   string
}

func (p *MockProvider) Create(name string, publicKeys []string, webhookUrl string) (string, uint16, error) {
	p.CreateCalled = true
	p.Name = name
	p.PublicKeys = publicKeys
	p.WebHookUrl = webhookUrl
	return "", 0, nil
}
