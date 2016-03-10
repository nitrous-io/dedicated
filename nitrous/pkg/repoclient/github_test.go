package repoclient

import "testing"

func TestGithubParseURL(t *testing.T) {
	testCases := []struct {
		url      string
		owner    string
		repoName string
		err      error
	}{
		{
			url:      "https://github.com/sferik/rails_admin",
			owner:    "sferik",
			repoName: "rails_admin",
		},
		{
			url:      "https://github.com/sferik/rails_admin.git",
			owner:    "sferik",
			repoName: "rails_admin",
		},
		{
			url: "https://github.com/website/sferik/rails_admin.git",
			err: ErrInvalidURLPath,
		},
		{
			url:      "git@github.com/sferik/rails_admin.git",
			owner:    "sferik",
			repoName: "rails_admin",
		},
		{
			url:      "git@github.com/sferik/rails_admin",
			owner:    "sferik",
			repoName: "rails_admin",
		},
		{
			url: "git@github.com/website/sferik/rails_admin.git",
			err: ErrInvalidURLPath,
		},
	}

	cl := NewGithubClient("some-token")

	for _, tc := range testCases {
		owner, repoName, err := cl.ParseURL(tc.url)
		if err != tc.err {
			t.Fatalf("expected ParseURL(%q) to fail with %v but got %v",
				tc.url, tc.err, err)
		}

		if owner != tc.owner {
			t.Fatalf("expected owner from ParseURL(%q) to be %q but got %q",
				tc.url, tc.owner, owner)
		}

		if repoName != tc.repoName {
			t.Fatalf("expected repo name from ParseURL(%q) to be %q but got %q",
				tc.url, tc.repoName, repoName)
		}
	}
}
