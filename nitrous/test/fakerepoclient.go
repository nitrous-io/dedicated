package test

type FakeRepoClient struct{}

func (s *FakeRepoClient) FileContent(owner, repo, file string) (string, error) {
	return `{
		"template": "rails",
		"ports": [8000, 9000, 10000]
	}`, nil
}

func (s *FakeRepoClient) ParseUrlPath(path string) (string, string, error) {
	return "owner", "repo-name", nil
}
