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
	validSlugRegex          *regexp.Regexp
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())

	nonAlphaNumericsRegex = regexp.MustCompile(`[^A-Za-z0-9\-]`)
	repeatedDashesRegex = regexp.MustCompile(`-{2,}`)
	dashesAtStartOrEndRegex = regexp.MustCompile(`^-|-$`)
	validSlugRegex = regexp.MustCompile(`^[a-z0-9\-]+-\d{4,}$`)
}

func random(min, max int) int {
	return rand.Intn(max-min) + min
}

func Build(s string) string {
	if len(s) == 0 {
		return ""
	}

	s = strings.ToLower(s)
	s = nonAlphaNumericsRegex.ReplaceAllString(s, "-")
	slug := fmt.Sprintf("%s-%v", s, random(1000, 9999))
	return sanitize(slug)
}

func BuildWithNumber(s string, number int) string {
	if len(s) == 0 {
		return ""
	}

	s = strings.ToLower(s)
	s = nonAlphaNumericsRegex.ReplaceAllString(s, "-")
	slug := fmt.Sprintf("%s-%v", s, number)
	return sanitize(slug)
}

// IsValid is intended to be used as a test helper
func IsValid(slug string) bool {
	return validSlugRegex.MatchString(slug)
}

func sanitize(slug string) string {
	slug = repeatedDashesRegex.ReplaceAllString(slug, "-")
	return dashesAtStartOrEndRegex.ReplaceAllString(slug, "")
}
