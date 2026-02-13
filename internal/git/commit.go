package git

import "fmt"

// FormatCommitMessage creates a conventional commit message with task metadata.
func FormatCommitMessage(title, taskID, authorName, authorEmail string) string {
	return fmt.Sprintf("feat(codeforge): %s\n\nTask ID: %s\nCo-authored-by: %s <%s>",
		title, taskID, authorName, authorEmail)
}
