package slugbuilder

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"
)

var (
	nonAlphaNumericsRegex   *regexp.Regexp
	repeatedDashesRegex     *regexp.Regexp
	dashesAtStartOrEndRegex *regexp.Regexp
	startsWithNumberRegex   *regexp.Regexp
	validSlugRegex          *regexp.Regexp

	// numericSlugPrefix is prefixed to slugs that end up starting with a number
	// after sanitization.
	numericSlugPrefix = "slug"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())

	nonAlphaNumericsRegex = regexp.MustCompile(`[^A-Za-z0-9\-]`)
	repeatedDashesRegex = regexp.MustCompile(`-{2,}`)
	dashesAtStartOrEndRegex = regexp.MustCompile(`^-|-$`)
	startsWithNumberRegex = regexp.MustCompile(`^[0-9]`)
	validSlugRegex = regexp.MustCompile(`^[a-z0-9\-]+-\d{4,}$`)
}

func random(min, max int) int {
	return rand.Intn(max-min) + min
}

func Build(s string) string {
	if len(s) == 0 {
		return ""
	}

	s = Slugify(s)
	slug := fmt.Sprintf("%s-%v", s, random(1000, 9999))
	return sanitize(slug)
}

func BuildWithNumber(s string, number int) string {
	if len(s) == 0 {
		return ""
	}

	s = Slugify(s)
	slug := fmt.Sprintf("%s-%v", s, number)
	return sanitize(slug)
}

func Slugify(s string) string {
	if len(s) == 0 {
		return ""
	}

	s = strings.ToLower(s)
	s = nonAlphaNumericsRegex.ReplaceAllString(s, "-")
	s = sanitize(s)

	// Prefix with non-numeric characters if slug starts with a number.
	if startsWithNumberRegex.MatchString(s) {
		s = numericSlugPrefix + s
	}

	if s == "" {
		s = numericSlugPrefix
	}

	return s
}

// IsValid is intended to be used as a test helper
func IsValid(slug string) bool {
	return validSlugRegex.MatchString(slug)
}

func sanitize(slug string) string {
	slug = repeatedDashesRegex.ReplaceAllString(slug, "-")
	return dashesAtStartOrEndRegex.ReplaceAllString(slug, "")
}
