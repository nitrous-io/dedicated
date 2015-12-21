package parsers

import "testing"

func TestParseStackNitrousImage(t *testing.T) {
	testCases := []struct {
		image    string
		expected string
	}{
		{
			image:    "quay.io/nitrous/rails:latest",
			expected: "rails",
		},
		{
			image:    "quay.io/nitrous/ubuntu:latest",
			expected: "ubuntu",
		},
		{
			image:    "quay.io/nitrous/jekyll:latest",
			expected: "jekyll",
		},
	}

	for _, tc := range testCases {
		got := ParseStack(tc.image)
		if got != tc.expected {
			t.Fatalf("expected stack of %q to be %q but got %q", tc.image, tc.expected, got)
		}
	}
}

func TestParseStackPatternMatching(t *testing.T) {
	testCases := []struct {
		image    string
		expected string
	}{
		{
			image:    "rails:4.2",
			expected: "rails",
		},
		{
			image:    "ruby-on-rails:latest",
			expected: "rails",
		},
		{
			image:    "django:1.9",
			expected: "django",
		},
		{
			image:    "python:2.6",
			expected: "python",
		},
		{
			image:    "wordpress-blog:latest",
			expected: "wordpress",
		},
		{
			image:    "php-foo:latest",
			expected: "php",
		},
		{
			image:    "meteor-dev:latest",
			expected: "meteor",
		},
		{
			image:    "nodejs:latest",
			expected: "nodejs",
		},
		{
			image:    "golang-tools:latest",
			expected: "golang",
		},
		{
			image:    "rust:latest",
			expected: "rust",
		},
		{
			image:    "docker:latest",
			expected: "docker",
		},
		{
			image:    "discourse-forum:latest",
			expected: "discourse",
		},
		{
			image:    "laravel:latest",
			expected: "laravel",
		},
		{
			image:    "html5-bootstrap:latest",
			expected: "html5",
		},
	}

	for _, tc := range testCases {
		got := ParseStack(tc.image)
		if got != tc.expected {
			t.Fatalf("expected stack of %q to be %q but got %q", tc.image, tc.expected, got)
		}
	}
}

func TestParseStackUnknown(t *testing.T) {
	testCases := []string{"busybox:latest", "scratch", "unknown", "fierce lannisport"}

	for _, tc := range testCases {
		if ParseStack(tc) != "" {
			t.Fatalf("expected stack of %q to be an empty string but got %q", tc, ParseStack(tc))
		}
	}
}
