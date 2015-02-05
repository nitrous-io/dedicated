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
		{"향기로운-바람-69", `69-\d{4,}$`},
	} {
		o := slugbuilder.Build(tc[0])
		if m, err := regexp.MatchString(tc[1], o); !m || err != nil {
			t.Errorf("%s is not a valid slug. input string: %s", o, tc[0])
		}
	}

}
