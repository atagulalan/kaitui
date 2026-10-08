package main

import (
	"strings"
	"testing"
)

func TestBuildAgentCommandEmpty(t *testing.T) {
	_, err := BuildAgentCommand("custom", nil, "", "  ")
	if err == nil {
		t.Fatal("expected empty prompt error")
	}
}

func TestBuildAgentPrompt(t *testing.T) {
	p := buildAgentPrompt("", "fix title")
	if !containsAll(p, "(none)", "fix title", "./kai", "140", "move <id>", "Do not run ./kai help") {
		t.Fatalf("p=%q", p)
	}
}

func TestAgentReplyLastLine(t *testing.T) {
	got := agentReply("noise\nmore noise\n\nMoved SpinaU001 to doing.\n")
	if got != "Moved SpinaU001 to doing." {
		t.Fatalf("got=%q", got)
	}
}

func TestAgentReplyCodexBlock(t *testing.T) {
	out := `OpenAI Codex v0.149.0
--------
user
You are editing this repo's kanban via ./kai only.
codex
Heyyo! What would you like to tackle in the kanban?
tokens used
3,197
Heyyo! What would you like to tackle in the kanban?
`
	got := agentReply(out)
	if got != "Heyyo! What would you like to tackle in the kanban?" {
		t.Fatalf("got=%q", got)
	}
}

func TestAgentReplyPrefersLastCodexBlock(t *testing.T) {
	out := `codex
Kanban panosundaki eleman sayısını kontrol ediyorum.
exec
./kai board
codex
Boardda toplam 3 eleman var: 1 todo, 1 doing ve 1 done.
tokens used
4,099
Boardda toplam 3 eleman var: 1 todo, 1 doing ve 1 done.
`
	got := agentReply(out)
	want := "Boardda toplam 3 eleman var: 1 todo, 1 doing ve 1 done."
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestAgentTranscriptIncludesSteps(t *testing.T) {
	out := `OpenAI Codex v0.149.0
--------
model: gpt-5.6-luna
user
prompt here
codex
Kontrol ediyorum.
exec
./kai board
board output
codex
Boardda 3 eleman var.
tokens used
4,099
Boardda 3 eleman var.
`
	got := agentTranscript(out)
	if !containsAll(got, "Kontrol ediyorum.", "./kai board", "board output", "Boardda 3 eleman var.") {
		t.Fatalf("got=%q", got)
	}
	if strings.Contains(got, "tokens used") || strings.Contains(got, "OpenAI Codex") {
		t.Fatalf("meta leaked: %q", got)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !stringContains(s, p) {
			return false
		}
	}
	return true
}

func stringContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
