package slugbuilder_test

import (
	"regexp"
	"testing"

	"github.com/nitrous-io/dedicated/pkg/slugbuilder"
)

func TestBuild(t *testing.T) {
	// test for empty string
	if slugbuilder.Build("") != "" {
		t.Errorf("slug of an empty string must be empty")
	}

	for _, tc := range [][]string{
		{"hello", `hello-\d{4,}$`},
		{"HELLO", `hello-\d{4,}$`},
		{"HellO", `hello-\d{4,}$`},
		{"hello-123", `hello-123-\d{4,}$`},
		{"-hello-", `hello-\d{4,}$`},
		{"he--llo", `he-llo-\d{4,}$`},
		{"hello_123", `hello-123-\d{4,}$`},
		{"hello@example.com", `hello-example-com-\d{4,}$`},
		{"hello world", `hello-world-\d{4,}$`},
		{"향기로운-바람-69", `slug69-\d{4,}$`},
		{"향기로운-바람", `slug-\d{4,}$`},
	} {
		o := slugbuilder.Build(tc[0])
		if m, err := regexp.MatchString(tc[1], o); !m || err != nil {
			t.Errorf("%s is not a valid slug. input string: %s", o, tc[0])
		}
	}
}

func TestBuildWithNumber(t *testing.T) {
	// test for empty string
	if slugbuilder.BuildWithNumber("", 1234) != "" {
		t.Errorf("slug of an empty string must be empty")
	}

	for _, tc := range [][]string{
		{"hello", "hello-1234"},
		{"HELLO", "hello-1234"},
		{"HellO", "hello-1234"},
		{"hello-123", "hello-123-1234"},
		{"-hello-", "hello-1234"},
		{"he--llo", "he-llo-1234"},
		{"hello_123", "hello-123-1234"},
		{"hello@example.com", "hello-example-com-1234"},
		{"hello world", "hello-world-1234"},
		{"향기로운-바람-69", "slug69-1234"},
		{"향기로운-바람", "slug-1234"},
	} {
		o := slugbuilder.BuildWithNumber(tc[0], 1234)
		if o != tc[1] {
			t.Errorf("%s is not a valid slug. input string: %s", o, tc[0])
		}
	}
}

func TestSlugify(t *testing.T) {
	// test for empty string
	if slugbuilder.Slugify("") != "" {
		t.Errorf("slug of an empty string must be empty")
	}

	for _, tc := range [][]string{
		{"hello", "hello"},
		{"HELLO", "hello"},
		{"HellO", "hello"},
		{"hello-123", "hello-123"},
		{"-hello-", "hello"},
		{"he--llo", "he-llo"},
		{"hello_123", "hello-123"},
		{"hello@example.com", "hello-example-com"},
		{"hello world", "hello-world"},
		{"향기로운-바람-69", "slug69"},
		{"향기로운-바람", "slug"},
		{"node4.2.1 12/3/2015 2:45:37 PM", "node4-2-1-12-3-2015-2-45-37-pm"},
	} {
		o := slugbuilder.Slugify(tc[0])
		if o != tc[1] {
			t.Errorf("%q is not a valid slug. input string: %q", o, tc[0])
		}
	}
}
