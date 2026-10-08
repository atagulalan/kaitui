package main

import "testing"

func TestParseSlash(t *testing.T) {
	cases := []struct {
		in   string
		cmd  string
		ok   bool
	}{
		{"/refresh", "/refresh", true},
		{"  /Quit  ", "/quit", true},
		{"/q", "/q", true},
		{"/help extra", "/help", true},
		{"move it", "", false},
		{"", "", false},
		{"refresh", "", false},
	}
	for _, c := range cases {
		cmd, ok := parseSlash(c.in)
		if ok != c.ok || cmd != c.cmd {
			t.Fatalf("parseSlash(%q)=(%q,%v) want (%q,%v)", c.in, cmd, ok, c.cmd, c.ok)
		}
	}
}

func TestWrapText(t *testing.T) {
	lines := wrapText("abcdefghij", 4)
	if len(lines) != 3 || lines[0] != "abcd" || lines[1] != "efgh" || lines[2] != "ij" {
		t.Fatalf("%v", lines)
	}
}

func TestOpenAIDetail(t *testing.T) {
	m := &model{
		aiLog: []aiEntry{{
			Summary: "Boardda toplam 3 eleman var.",
			Detail:  "$\n./kai board\n\n›\nBoardda toplam 3 eleman var.",
		}},
		pressAIIdx: -1,
	}
	_, _ = m.openAIDetail(0)
	if !m.detailOpen || m.detailID != "agent" || !stringContains(m.detailBody, "./kai board") {
		t.Fatalf("open=%v id=%q body=%q", m.detailOpen, m.detailID, m.detailBody)
	}
}

func TestRunSlash(t *testing.T) {
	m := &model{
		selected: map[string]bool{"A": true},
		status:   "",
	}
	_, cmd := m.runSlash("/clear")
	if cmd != nil {
		t.Fatal("clear should not return tea.Cmd")
	}
	if len(m.selected) != 0 {
		t.Fatalf("selected=%v", m.selected)
	}
	_, cmd = m.runSlash("/help")
	if cmd != nil || !stringContains(m.status, "/refresh") {
		t.Fatalf("help status=%q", m.status)
	}
	_, cmd = m.runSlash("/nope")
	if cmd != nil || !stringContains(m.status, "unknown") {
		t.Fatalf("unknown status=%q", m.status)
	}
}
