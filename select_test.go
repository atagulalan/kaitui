package main

import "testing"

func TestToggleSelect(t *testing.T) {
	m := &model{
		board: Board{
			Columns: []string{"todo", "doing"},
			Cards: map[string][]Card{
				"todo":  {{ID: "A"}, {ID: "B"}, {ID: "C"}},
				"doing": {{ID: "D"}},
			},
		},
		agentCfg: AgentConfig{Columns: []string{"todo", "doing"}},
		selected: map[string]bool{},
	}

	m.toggleSelect("A")
	if !m.selected["A"] || len(m.selected) != 1 {
		t.Fatalf("add A: %v", m.selected)
	}

	m.toggleSelect("C")
	if !m.selected["A"] || !m.selected["C"] || len(m.selected) != 2 {
		t.Fatalf("add C: %v", m.selected)
	}

	m.toggleSelect("B")
	if len(m.selected) != 3 || !m.selected["B"] {
		t.Fatalf("add B: %v", m.selected)
	}

	m.toggleSelect("C")
	if m.selected["C"] || !m.selected["A"] || !m.selected["B"] {
		t.Fatalf("toggle off C: %v", m.selected)
	}

	m.clearSelection()
	if len(m.selected) != 0 {
		t.Fatalf("clear: %v", m.selected)
	}
}

func TestBuildAgentPromptMulti(t *testing.T) {
	p := buildAgentPrompt("A,B", "move both")
	if !stringContains(p, "Selected cards: A,B") {
		t.Fatalf("p=%q", p)
	}
}
