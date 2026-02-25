package git

import "testing"

func TestParseShortStat(t *testing.T) {
	tests := []struct {
		input string
		ins   int
		del   int
	}{
		{"3 files changed, 142 insertions(+), 38 deletions(-)\n", 142, 38},
		{"1 file changed, 1 insertion(+), 1 deletion(-)\n", 1, 1},
		{"1 file changed, 5 insertions(+)\n", 5, 0},
		{"1 file changed, 3 deletions(-)\n", 0, 3},
		{"", 0, 0},
		{"nothing", 0, 0},
	}

	for _, tt := range tests {
		ins, del := parseShortStat(tt.input)
		if ins != tt.ins || del != tt.del {
			t.Errorf("parseShortStat(%q) = (%d, %d), want (%d, %d)", tt.input, ins, del, tt.ins, tt.del)
		}
	}
}
