package workspace

import (
	"github.com/freema/codeforge/internal/slug"
)

// GenerateSlug creates a human-readable directory name from a prompt and task ID.
// Format: "first-five-words-{shortTaskID}" (max 50 chars).
func GenerateSlug(prompt, taskID string) string {
	return slug.Generate(prompt, taskID)
}
