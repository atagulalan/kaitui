package main

import "testing"

func TestApplyResize(t *testing.T) {
	m := &model{
		colWidths: []int{30, 30, 30},
		resizing:  true,
		resizeIdx: 0,
		resizeX0:  30,
		resizeW0:  30,
		resizeN0:  30,
	}
	m.applyResize(40) // +10 to left
	if m.colWidths[0] != 40 || m.colWidths[1] != 20 {
		t.Fatalf("got %v", m.colWidths)
	}
	m.resizeX0 = 40
	m.resizeW0 = m.colWidths[0]
	m.resizeN0 = m.colWidths[1]
	m.applyResize(0) // pull far left → clamp
	if m.colWidths[0] < minColWidth || m.colWidths[1] < minColWidth {
		t.Fatalf("below min: %v", m.colWidths)
	}
}
