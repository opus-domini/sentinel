package recovery

import "context"

// layoutNode represents a node in a parsed tmux window layout tree.
//
// tmux layout strings follow this grammar (from #{window_layout}):
//
//	leaf:    checksum,WxH,x,y,pane_id
//	vsplit:  checksum,WxH,x,y{child,child,...}   → vertical dividers (side-by-side columns)
//	hsplit:  checksum,WxH,x,y[child,child,...]   → horizontal dividers (stacked rows)
//
// SplitDir indicates the container direction:
//
//	'{' → vertical split (columns), creates panes with tmux split-window -h
//	'[' → horizontal split (rows),  creates panes with tmux split-window -v
//	 0  → leaf node (a single pane)
type layoutNode struct {
	SplitDir byte          // '{', '[', or 0 for leaf
	Children []*layoutNode // non-nil only for branch nodes
}

// leafCount returns the total number of leaf (pane) nodes in the tree.
func (n *layoutNode) leafCount() int {
	if n == nil {
		return 0
	}
	if n.SplitDir == 0 {
		return 1
	}
	total := 0
	for _, child := range n.Children {
		total += child.leafCount()
	}
	return total
}

// parseLayout parses a tmux layout string and returns the root node.
// Returns nil if the string is empty or malformed.
func parseLayout(raw string) *layoutNode {
	if raw == "" {
		return nil
	}
	// Skip the leading "checksum," prefix (e.g. "83ed,").
	pos := 0
	for pos < len(raw) && raw[pos] != ',' {
		pos++
	}
	if pos >= len(raw) {
		return nil
	}
	pos++ // skip ','
	node, end := parseLayoutNode(raw, pos)
	if node == nil || end != len(raw) {
		return nil
	}
	return node
}

// parseLayoutNode parses a layout node starting at position pos.
// Returns the parsed node and the position after parsing.
//
// Each node starts with "WxH,x,y" (the geometry, 2 commas), then either:
//   - ",pane_id"   → leaf
//   - "{children}" → vertical split
//   - "[children]" → horizontal split
func parseLayoutNode(raw string, pos int) (*layoutNode, int) {
	if pos >= len(raw) {
		return nil, pos
	}

	// Skip "WxH,x,y" — exactly 2 commas separating three geometry fields.
	commas := 0
	for pos < len(raw) && commas < 2 {
		if raw[pos] == ',' {
			commas++
		}
		pos++
	}
	if commas < 2 {
		return nil, pos
	}
	// Consume the y value (digits after the second comma).
	for pos < len(raw) && raw[pos] >= '0' && raw[pos] <= '9' {
		pos++
	}
	if pos >= len(raw) {
		return nil, pos
	}

	switch raw[pos] {
	case '{', '[':
		closer := byte('}')
		if raw[pos] == '[' {
			closer = ']'
		}
		node := &layoutNode{SplitDir: raw[pos]}
		pos++ // skip opener

		for {
			child, end := parseLayoutNode(raw, pos)
			if child == nil {
				return nil, end
			}
			node.Children = append(node.Children, child)
			pos = end
			if pos >= len(raw) {
				return nil, pos
			}
			if raw[pos] == closer {
				pos++ // skip closer
				return node, pos
			}
			if raw[pos] == ',' {
				pos++ // skip comma between children
				continue
			}
			return nil, pos
		}

	case ',':
		pos++ // skip comma before pane_id
		start := pos
		for pos < len(raw) && raw[pos] >= '0' && raw[pos] <= '9' {
			pos++
		}
		if pos == start {
			return nil, pos
		}
		return &layoutNode{SplitDir: 0}, pos

	default:
		return nil, pos
	}
}

// tmuxSplitDirection returns the SplitPaneIn direction string
// for a given container SplitDir byte.
func tmuxSplitDirection(splitDir byte) string {
	if splitDir == '{' {
		return "vertical" // '{' = columns → SplitPaneIn "vertical" → tmux -h
	}
	return "horizontal" // '[' = rows → SplitPaneIn "horizontal" → tmux -v
}

// buildPanesResult holds the output of buildPanes.
type buildPanesResult struct {
	paneIDs []string // leaf pane IDs in DFS order matching snapshot pane order
}

// buildPanes creates the pane tree structure by splitting panes according to the
// layout tree. It returns pane IDs in DFS (leaf) order, matching the snapshot's
// pane ordering. The caller supplies the initial paneID (the one already
// existing in the window) and a cwdFn that maps leaf index → cwd path.
func buildPanes(ctx context.Context, tmux splitPaner, node *layoutNode, paneID string,
	cwdFn func(int) string, counter *int) (buildPanesResult, error) {
	if node == nil {
		return buildPanesResult{}, nil
	}
	if node.SplitDir == 0 {
		// Leaf: this pane already exists.
		idx := *counter
		*counter++
		_ = idx // cwdFn already applied at split time
		return buildPanesResult{paneIDs: []string{paneID}}, nil
	}

	// Branch: split the current pane for each child after the first.
	direction := tmuxSplitDirection(node.SplitDir)
	childPaneIDs := make([]string, len(node.Children))
	childPaneIDs[0] = paneID // first child inherits the existing pane

	for i := 1; i < len(node.Children); i++ {
		cwd := cwdFn(*counter + leafCountBefore(node, i))
		newID, err := tmux.SplitPaneIn(ctx, paneID, direction, cwd)
		if err != nil {
			return buildPanesResult{}, err
		}
		childPaneIDs[i] = newID
	}

	// Recurse into each child to build sub-trees.
	var allLeafIDs []string
	for i, child := range node.Children {
		result, err := buildPanes(ctx, tmux, child, childPaneIDs[i], cwdFn, counter)
		if err != nil {
			return buildPanesResult{}, err
		}
		allLeafIDs = append(allLeafIDs, result.paneIDs...)
	}
	return buildPanesResult{paneIDs: allLeafIDs}, nil
}

// splitPaner is the minimal interface needed by buildPanes.
type splitPaner interface {
	SplitPaneIn(ctx context.Context, paneID, direction, cwd string) (string, error)
}

// leafCountBefore counts how many leaves exist in children[0..idx-1] of a node.
func leafCountBefore(node *layoutNode, idx int) int {
	total := 0
	for i := 0; i < idx; i++ {
		total += node.Children[i].leafCount()
	}
	return total
}
