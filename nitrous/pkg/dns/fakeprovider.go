package dns

import "sync"

type FakeProvider struct {
	receivedRequests []FakeRequest
	mu               sync.Mutex
}

type FakeRequest struct {
	Name string
	Args []interface{}
}

func (p *FakeProvider) ReceivedRequests() []FakeRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.receivedRequests
}

func (p *FakeProvider) SetCNAME(src, dst string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.receivedRequests = append(p.receivedRequests, FakeRequest{
		Name: "SetCNAME",
		Args: []interface{}{src, dst},
	})
	return nil
}

func (p *FakeProvider) DeleteCNAME(domain string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.receivedRequests = append(p.receivedRequests, FakeRequest{
		Name: "DeleteCNAME",
		Args: []interface{}{domain},
	})
	return nil
}
