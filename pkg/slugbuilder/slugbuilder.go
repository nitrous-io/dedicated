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

	Builder func(string) string
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())

	nonAlphaNumericsRegex = regexp.MustCompile(`[^A-Za-z0-9\-]`)
	repeatedDashesRegex = regexp.MustCompile(`-{2,}`)
	dashesAtStartOrEndRegex = regexp.MustCompile(`^-|-$`)
	validSlugRegex = regexp.MustCompile(`^[a-z0-9\-]+-\d{4,}$`)

	Builder = random4
}

func random(min, max int) int {
	return rand.Intn(max-min) + min
}

func random4(n string) string {
	n = strings.ToLower(n)
	n = nonAlphaNumericsRegex.ReplaceAllString(n, "-")
	slug := fmt.Sprintf("%s-%v", n, random(1000, 9999))
	slug = repeatedDashesRegex.ReplaceAllString(slug, "-")
	return dashesAtStartOrEndRegex.ReplaceAllString(slug, "")
}

func Build(n string) string {
	if len(n) == 0 {
		return ""
	}
	return Builder(n)
}

// IsValid is intended to be used as a test helper
func IsValid(slug string) bool {
	return validSlugRegex.MatchString(slug)
}
