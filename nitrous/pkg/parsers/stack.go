package parsers

import (
	"regexp"
	"strings"
)

var nitrousImagePattern = regexp.MustCompile(`nitrous\/([^:]+):`)

var stackPatterns = []struct {
	needle string
	stack  string
}{
	{"rails", "rails"},
	{"ruby", "ruby"},
	{"django", "django"},
	{"python", "python"},
	{"wordpress", "wordpress"},
	{"php", "php"},
	{"meteor", "meteor"},
	{"nodejs", "nodejs"},
	{"golang", "golang"},
	{"rust", "rust"},
	{"docker", "docker"},
	{"discourse", "discourse"},
	{"laravel", "laravel"},
	{"html5", "html5"},
}

func ParseStack(str string) string {
	if submatches := nitrousImagePattern.FindStringSubmatch(str); len(submatches) == 2 {
		return submatches[1]
	}

	for _, sp := range stackPatterns {
		if strings.Contains(str, sp.needle) {
			return sp.stack
		}
	}
	return ""
}
