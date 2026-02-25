package slug

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		taskID string
		want   string
	}{
		{
			name:   "basic prompt",
			prompt: "Fix the failing auth tests",
			taskID: "550e8400-e29b-41d4-a716-446655440000",
			want:   "fix-the-failing-auth-tests-550e8400",
		},
		{
			name:   "empty prompt",
			prompt: "",
			taskID: "550e8400-e29b-41d4-a716-446655440000",
			want:   "task-550e8400",
		},
		{
			name:   "unicode accents",
			prompt: "Opravit chybu v autentizaci",
			taskID: "abcdef1234567890",
			want:   "opravit-chybu-v-autentizaci-abcdef12",
		},
		{
			name:   "more than 5 words",
			prompt: "Fix the very broken auth tests in the login module",
			taskID: "12345678abcdefgh",
			want:   "fix-the-very-broken-auth-12345678",
		},
		{
			name:   "special characters",
			prompt: "Add @middleware & fix #123 bug!",
			taskID: "aabbccdd11223344",
			want:   "add-middleware-fix-123-bug-aabbccdd",
		},
		{
			name:   "whitespace only",
			prompt: "   ",
			taskID: "aabbccdd11223344",
			want:   "task-aabbccdd",
		},
		{
			name:   "short task ID",
			prompt: "Fix bug",
			taskID: "abc",
			want:   "fix-bug-abc",
		},
		{
			name:   "long single word",
			prompt: "Superlongwordthatexceedsthelimitofcharacters",
			taskID: "12345678abcdefgh",
			want:   "superlongwordthatexceedsthelimitofcharact-12345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Generate(tt.prompt, tt.taskID)
			if got != tt.want {
				t.Errorf("Generate(%q, %q) = %q, want %q", tt.prompt, tt.taskID, got, tt.want)
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
