package test

type FakeRepoClient struct {
	Content string
}

func (c *FakeRepoClient) FileContent(owner, repo, file string) (string, error) {
	if c.Content != "" {
		return c.Content, nil
	}
	return `{
		"template": "rails",
		"ports": [8000, 9000, 10000]
	}`, nil
}

func (c *FakeRepoClient) ParseURL(u string) (string, string, error) {
	return "owner", "repo-name", nil
}

func (c *FakeRepoClient) OwnerAvatar(owner, repo string) (string, error) {
	return "https://example.com/rails.png", nil
}
