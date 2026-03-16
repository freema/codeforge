package git

import (
	"testing"
)

func TestParsePatch(t *testing.T) {
	tests := []struct {
		name     string
		patch    string
		expected map[int]bool
	}{
		{
			name:     "empty patch",
			patch:    "",
			expected: map[int]bool{},
		},
		{
			name: "simple addition",
			patch: `@@ -0,0 +1,3 @@
+line one
+line two
+line three`,
			expected: map[int]bool{1: true, 2: true, 3: true},
		},
		{
			name: "mixed add delete context",
			patch: `@@ -10,6 +10,7 @@
 context line
-deleted line
+added line one
+added line two
 another context`,
			expected: map[int]bool{
				10: true, // context
				11: true, // added line one
				12: true, // added line two
				13: true, // another context
			},
		},
		{
			name: "multiple hunks",
			patch: `@@ -1,3 +1,4 @@
 first
+inserted
 second
 third
@@ -20,3 +21,3 @@
 alpha
-beta
+gamma
 delta`,
			expected: map[int]bool{
				1: true, 2: true, 3: true, 4: true, // first hunk
				21: true, 22: true, 23: true, // second hunk
			},
		},
		{
			name: "no newline at end of file",
			patch: `@@ -1,2 +1,2 @@
-old line
+new line
 context
\ No newline at end of file`,
			expected: map[int]bool{1: true, 2: true},
		},
		{
			name: "new file",
			patch: `@@ -0,0 +1,5 @@
+package main
+
+func main() {
+	println("hello")
+}`,
			expected: map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true},
		},
		{
			name: "deletion only",
			patch: `@@ -1,3 +1,1 @@
-removed one
-removed two
 kept`,
			expected: map[int]bool{1: true},
		},
		{
			name: "hunk header without count",
			patch: `@@ -5 +5 @@
-old
+new`,
			expected: map[int]bool{5: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePatch(tt.patch)

			if len(got) != len(tt.expected) {
				t.Errorf("ParsePatch() returned %d lines, want %d\ngot:  %v\nwant: %v",
					len(got), len(tt.expected), got, tt.expected)
				return
			}

			for line, want := range tt.expected {
				if got[line] != want {
					t.Errorf("ParsePatch() line %d = %v, want %v", line, got[line], want)
				}
			}

			// Check no extra lines
			for line := range got {
				if !tt.expected[line] {
					t.Errorf("ParsePatch() unexpected line %d in result", line)
				}
			}
		})
	}
}

func TestDiffLineSet_Contains(t *testing.T) {
	d := DiffLineSet{
		"main.go": {10: true, 11: true, 12: true},
		"util.go": {5: true},
	}

	tests := []struct {
		file string
		line int
		want bool
	}{
		{"main.go", 10, true},
		{"main.go", 13, false},
		{"util.go", 5, true},
		{"util.go", 6, false},
		{"missing.go", 1, false},
	}

	for _, tt := range tests {
		got := d.Contains(tt.file, tt.line)
		if got != tt.want {
			t.Errorf("DiffLineSet.Contains(%q, %d) = %v, want %v", tt.file, tt.line, got, tt.want)
		}
	}
}
