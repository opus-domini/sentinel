package recovery

import "testing"

func TestParseLayout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantLeafs int
		wantDir   byte
	}{
		{
			name:      "single pane (leaf)",
			input:     "8502,204x51,0,0,2",
			wantLeafs: 1,
			wantDir:   0,
		},
		{
			name:      "two columns (vertical split)",
			input:     "83ed,204x51,0,0{102x51,0,0,0,101x51,103,0,1}",
			wantLeafs: 2,
			wantDir:   '{',
		},
		{
			name:      "two rows (horizontal split)",
			input:     "a1b2,204x51,0,0[204x25,0,0,3,204x25,0,26,4]",
			wantLeafs: 2,
			wantDir:   '[',
		},
		{
			name:      "nested: vsplit with hsplit child",
			input:     "d5e6,204x51,0,0{102x51,0,0,5,101x51,103,0[101x25,103,0,6,101x25,103,26,7]}",
			wantLeafs: 3,
			wantDir:   '{',
		},
		{
			name:      "three columns",
			input:     "f0f0,204x51,0,0{68x51,0,0,10,67x51,69,0,11,67x51,137,0,12}",
			wantLeafs: 3,
			wantDir:   '{',
		},
		{
			name:    "empty string",
			input:   "",
			wantNil: true,
		},
		{
			name:    "malformed - no comma",
			input:   "abcd",
			wantNil: true,
		},
		{
			name:    "malformed - truncated geometry",
			input:   "abcd,100x50",
			wantNil: true,
		},
		{
			name:    "malformed - unclosed brace",
			input:   "83ed,204x51,0,0{102x51,0,0,0",
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			node := parseLayout(tc.input)
			if tc.wantNil {
				if node != nil {
					t.Fatalf("expected nil, got %+v", node)
				}
				return
			}
			if node == nil {
				t.Fatal("expected non-nil node")
			}
			if got := node.leafCount(); got != tc.wantLeafs {
				t.Errorf("leafCount = %d, want %d", got, tc.wantLeafs)
			}
			if node.SplitDir != tc.wantDir {
				t.Errorf("splitDir = %c, want %c", node.SplitDir, tc.wantDir)
			}
		})
	}
}

func TestLeafCountNil(t *testing.T) {
	t.Parallel()
	var n *layoutNode
	if got := n.leafCount(); got != 0 {
		t.Errorf("nil leafCount = %d, want 0", got)
	}
}

func TestTmuxSplitDirection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		dir  byte
		want string
	}{
		{'{', "vertical"},
		{'[', "horizontal"},
		{0, "horizontal"},
	}
	for _, tc := range tests {
		t.Run(string(rune(tc.dir)), func(t *testing.T) {
			t.Parallel()
			if got := tmuxSplitDirection(tc.dir); got != tc.want {
				t.Errorf("tmuxSplitDirection(%c) = %q, want %q", tc.dir, got, tc.want)
			}
		})
	}
}
