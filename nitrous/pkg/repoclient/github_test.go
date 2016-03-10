package repoclient

import "testing"

func TestGithubParseUrlPath(t *testing.T) {
	testCases := []struct {
		path     string
		owner    string
		repoName string
		err      error
	}{
		{
			path:     "/sferik/rails_admin",
			owner:    "sferik",
			repoName: "rails_admin",
		},
		{
			path:     "/sferik/rails_admin.git",
			owner:    "sferik",
			repoName: "rails_admin",
		},
		{
			path: "/website/sferik/rails_admin.git",
			err:  ErrInvalidUrlPath,
		},
	}

	cl := NewGithubClient("some-token")

	for _, tc := range testCases {
		owner, repoName, err := cl.ParseUrlPath(tc.path)
		if err != tc.err {
			t.Fatalf("expected ParseUrlPath(%q) to fail with %v but got %v",
				tc.path, tc.err, err)
		}

		if owner != tc.owner {
			t.Fatalf("expected owner from ParseUrlPath(%q) to be %q but got %q",
				tc.path, tc.owner, owner)
		}

		if repoName != tc.repoName {
			t.Fatalf("expected repo name from ParseUrlPath(%q) to be %q but got %q",
				tc.path, tc.repoName, repoName)
		}
	}
}
