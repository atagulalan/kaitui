package main

import (
	"strings"
	"testing"
)

func TestParseListOutput(t *testing.T) {
	in := `## todo
- [SpinaA002] p20 Needs foundation [blocked: SpinaU001]
- [SpinaU001] p10 Foundation [ready]
- [SpinaA001] p8 Parallel A [ready]

## doing
  (empty)

## done
- [SpinaA003] p1 wf-test
`
	b := ParseListOutput(in)
	if len(b.Columns) != 3 {
		t.Fatalf("columns=%v", b.Columns)
	}
	todo := b.Cards["todo"]
	if len(todo) != 3 {
		t.Fatalf("todo cards=%d", len(todo))
	}
	if todo[0].ID != "SpinaA002" || todo[0].Status != "blocked" {
		t.Fatalf("card0=%+v", todo[0])
	}
	if todo[1].Title != "Foundation" || todo[1].Status != "ready" {
		t.Fatalf("card1=%+v", todo[1])
	}
	if len(b.Cards["doing"]) != 0 {
		t.Fatalf("doing should be empty")
	}
	done := b.Cards["done"]
	if len(done) != 1 || done[0].ID != "SpinaA003" || done[0].Note != "" {
		t.Fatalf("done=%+v", done)
	}
}

func TestBuildAgentCommand(t *testing.T) {
	tmpls := map[string]string{
		"cursor": "cursor agent -p --force {prompt}",
	}
	_, err := BuildAgentCommand("none", tmpls, "SpinaA001", "hi")
	if err == nil {
		t.Fatal("expected error for unknown/none template")
	}
	cmd, err := BuildAgentCommand("cursor", tmpls, "SpinaA001", "move to doing")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd, "cursor agent") {
		t.Fatalf("cmd=%s", cmd)
	}
	if !strings.Contains(cmd, "Selected card: SpinaA001") {
		t.Fatalf("prompt missing id: %s", cmd)
	}
	custom, err := BuildAgentCommand("custom", tmpls, "X", "echo hello")
	if err != nil {
		t.Fatal(err)
	}
	if custom != "echo hello" {
		t.Fatalf("custom=%q", custom)
	}
}

func TestAgentNames(t *testing.T) {
	cfg := AgentConfig{
		Default: "none",
		Templates: map[string]string{
			"codex":  "x",
			"cursor": "y",
			"claude": "z",
			"other":  "w",
		},
	}
	names := AgentNames(cfg)
	wantPrefix := []string{"none", "cursor", "claude", "codex"}
	for i, w := range wantPrefix {
		if names[i] != w {
			t.Fatalf("names=%v want prefix %v", names, wantPrefix)
		}
	}
	if names[len(names)-1] != "custom" {
		t.Fatalf("last=%v", names)
	}
}

func TestShellQuote(t *testing.T) {
	q := shellQuote("it's")
	if !strings.Contains(q, "'\"'\"'") && q != `'it'"'"'s'` {
		// acceptable forms
		if !strings.HasPrefix(q, "'") {
			t.Fatalf("q=%q", q)
		}
	}
}
