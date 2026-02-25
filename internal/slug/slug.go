package slug

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	maxSlugLen = 50
	maxWords   = 5
	shortIDLen = 8
)

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// Generate creates a human-readable slug from a prompt and task ID.
// Format: "first-five-words-{shortTaskID}" (max 50 chars).
func Generate(prompt, taskID string) string {
	shortID := taskID
	if len(shortID) > shortIDLen {
		shortID = shortID[:shortIDLen]
	}

	s := Slugify(prompt)
	if s == "" {
		return "task-" + shortID
	}

	// Truncate slug to fit within maxSlugLen including "-{shortID}" suffix
	suffix := "-" + shortID
	maxBase := maxSlugLen - len(suffix)
	if maxBase < 1 {
		return "task-" + shortID
	}

	if len(s) > maxBase {
		s = s[:maxBase]
		// Don't end on a hyphen
		s = strings.TrimRight(s, "-")
	}

	return s + suffix
}

// Slugify converts a string to a URL-safe slug using the first N words.
func Slugify(s string) string {
	// Normalize unicode to ASCII-compatible decomposed form
	s = norm.NFKD.String(s)

	// Remove non-ASCII characters (accents, etc.)
	var b strings.Builder
	for _, r := range s {
		if r < unicode.MaxASCII {
			b.WriteRune(r)
		}
	}
	s = b.String()

	s = strings.ToLower(s)

	// Replace non-alphanumeric sequences with hyphens
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	// Take first N words (hyphen-separated segments)
	parts := strings.SplitN(s, "-", maxWords+1)
	if len(parts) > maxWords {
		parts = parts[:maxWords]
	}

	return strings.Join(parts, "-")
}
