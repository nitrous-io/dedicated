package test

type FakeRepoClient struct{}

func (c *FakeRepoClient) FileContent(owner, repo, file string) (string, error) {
	return `{
		"template": "rails",
		"ports": [8000, 9000, 10000]
	}`, nil
}

func (c *FakeRepoClient) ParseURL(u string) (string, string, error) {
	return "owner", "repo-name", nil
}

func (c *FakeRepoClient) OwnerAvatar(owner, repo string) (string, error) {
	return "owner", "https://example.com/rails.png"
}
