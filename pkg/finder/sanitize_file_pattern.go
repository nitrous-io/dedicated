package finder

import (
	"regexp"
	"strings"
)

// FixupFilePattern tries to convert a regexp entered by a user to a valid Go regexp.
// People are used to simplified syntax eg. "*.go", which is widely accepted
// by text editors like atom or Sublime. Those are often not valid according
// to Go regexp syntax. We use heuristics for most common cases. We guarantee
// to return a valid Go regexp (in the worst case it just escapes regexp meta-chars
// Note: this might need to be extended based on feedback from real life use
func SanitizeFilePattern(s string) string {
	_, err := regexp.Compile(s)
	if err == nil {
		return s
	}
	// most common is using "*" to mean ".*"
	s2 := strings.Replace(s, "*", ".*", -1)
	s2 = strings.Replace(s2, `*.`, `*\.`, -1)
	if _, err = regexp.Compile(s2); err == nil {
		return s2
	}
	return regexp.QuoteMeta(s)
}
