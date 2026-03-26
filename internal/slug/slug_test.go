package slug

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		sessionID string
		want   string
	}{
		{
			name:   "basic prompt",
			prompt: "Fix the failing auth tests",
			sessionID: "550e8400-e29b-41d4-a716-446655440000",
			want:   "fix-the-failing-auth-tests-550e8400",
		},
		{
			name:   "empty prompt",
			prompt: "",
			sessionID: "550e8400-e29b-41d4-a716-446655440000",
			want:   "session-550e8400",
		},
		{
			name:   "unicode accents",
			prompt: "Opravit chybu v autentizaci",
			sessionID: "abcdef1234567890",
			want:   "opravit-chybu-v-autentizaci-abcdef12",
		},
		{
			name:   "more than 5 words",
			prompt: "Fix the very broken auth tests in the login module",
			sessionID: "12345678abcdefgh",
			want:   "fix-the-very-broken-auth-12345678",
		},
		{
			name:   "special characters",
			prompt: "Add @middleware & fix #123 bug!",
			sessionID: "aabbccdd11223344",
			want:   "add-middleware-fix-123-bug-aabbccdd",
		},
		{
			name:   "whitespace only",
			prompt: "   ",
			sessionID: "aabbccdd11223344",
			want:   "session-aabbccdd",
		},
		{
			name:   "short session ID",
			prompt: "Fix bug",
			sessionID: "abc",
			want:   "fix-bug-abc",
		},
		{
			name:   "long single word",
			prompt: "Superlongwordthatexceedsthelimitofcharacters",
			sessionID: "12345678abcdefgh",
			want:   "superlongwordthatexceedsthelimitofcharact-12345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Generate(tt.prompt, tt.sessionID)
			if got != tt.want {
				t.Errorf("Generate(%q, %q) = %q, want %q", tt.prompt, tt.sessionID, got, tt.want)
			}
		})
	}
}

func TestGenerateMaxLength(t *testing.T) {
	s := Generate(strings.Repeat("word ", 100), "12345678abcdefgh")
	if len(s) > maxSlugLen {
		t.Errorf("slug length %d exceeds max %d: %q", len(s), maxSlugLen, s)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "Fix the auth bug", "fix-the-auth-bug"},
		{"czech", "Přidat novou funkci", "pridat-novou-funkci"},
		{"special chars", "Add @middleware!", "add-middleware"},
		{"empty", "", ""},
		{"max words", "one two three four five six", "one-two-three-four-five"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
