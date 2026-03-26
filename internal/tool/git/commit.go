package git

import "fmt"

// FormatCommitMessage creates a conventional commit message with session metadata.
func FormatCommitMessage(title, sessionID, authorName, authorEmail string) string {
	return fmt.Sprintf("feat(codeforge): %s\n\nSession ID: %s\nCo-authored-by: %s <%s>",
		title, sessionID, authorName, authorEmail)
}
