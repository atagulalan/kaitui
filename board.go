package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Card is one kanban item as shown by `./kai list --all`.
type Card struct {
	ID       string
	Priority string
	Title    string
	Note     string // e.g. [ready] or [blocked: X] from list
	Status   string // ready | blocked | ""
	Created  string // YYYY-MM-DD from item frontmatter
	Column   string
}

// Board holds columns and cards.
type Board struct {
	Columns []string
	Cards   map[string][]Card // column -> cards
}

// AgentConfig is TUI agent settings from .kai/config.
type AgentConfig struct {
	Default   string            // none | name
	Templates map[string]string // name -> shell template
	Columns   []string
}

var (
	reColumn = regexp.MustCompile(`^##\s+(\S+)\s*$`)
	reCard   = regexp.MustCompile(`^- \[([^\]]+)\] p(-?\d+)\s+(.*)$`)
	reNote   = regexp.MustCompile(`\s+(\[(?:ready|blocked:[^\]]+)\])\s*$`)
)

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		cfg := filepath.Join(dir, ".kai", "config")
		if fileExists(cfg) {
			return dir, nil
		}
		// consumer: ./kai file, or monorepo: kai/kai
		if isExecFile(filepath.Join(dir, "kai")) || isExecFile(filepath.Join(dir, "kai", "kai")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("kai repo root not found (no ./kai or .kai/config)")
		}
		dir = parent
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isExecFile(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode()&0o111 != 0
}

func kaiBin(root string) string {
	for _, c := range []string{
		filepath.Join(root, "kai"),
		filepath.Join(root, "kai", "kai"),
		filepath.Join(root, ".kai", "kai"),
	} {
		if isExecFile(c) {
			return c
		}
	}
	return filepath.Join(root, "kai")
}

func runKai(root string, args ...string) (string, error) {
	bin := kaiBin(root)
	cmd := exec.Command(bin, args...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

// ParseListOutput parses `./kai list --all` stdout into a Board.
func ParseListOutput(out string) Board {
	b := Board{Cards: map[string][]Card{}}
	var col string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if m := reColumn.FindStringSubmatch(line); m != nil {
			col = m[1]
			if _, ok := b.Cards[col]; !ok {
				b.Columns = append(b.Columns, col)
				b.Cards[col] = nil
			}
			continue
		}
		if col == "" {
			continue
		}
		if strings.TrimSpace(line) == "(empty)" {
			continue
		}
		m := reCard.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		title := m[3]
		note := ""
		if nm := reNote.FindStringSubmatch(title); nm != nil {
			note = nm[1]
			title = strings.TrimSpace(reNote.ReplaceAllString(title, ""))
		}
		c := Card{
			ID:       m[1],
			Priority: m[2],
			Title:    title,
			Note:     note,
			Column:   col,
		}
		c.Status = statusFromNote(note)
		b.Cards[col] = append(b.Cards[col], c)
	}
	return b
}

func statusFromNote(note string) string {
	switch {
	case strings.HasPrefix(note, "[ready]"):
		return "ready"
	case strings.HasPrefix(note, "[blocked"):
		return "blocked"
	default:
		return ""
	}
}

func readItemCreated(root, id string) string {
	path := filepath.Join(root, ".kai", "items", id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	inFM := false
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			if !inFM {
				inFM = true
				continue
			}
			break
		}
		if !inFM {
			continue
		}
		if key, val, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(key) == "created" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func enrichBoard(root string, b *Board) {
	for col, cards := range b.Cards {
		for i := range cards {
			if cards[i].Created == "" {
				cards[i].Created = readItemCreated(root, cards[i].ID)
			}
			if cards[i].Status == "" {
				cards[i].Status = statusFromNote(cards[i].Note)
			}
		}
		b.Cards[col] = cards
	}
}

func LoadBoard(root string) (Board, error) {
	out, err := runKai(root, "list", "--all")
	if err != nil {
		return Board{}, err
	}
	b := ParseListOutput(out)
	enrichBoard(root, &b)
	return b, nil
}

// LoadAgentConfig reads agent= and agent.*= from .kai/config.
func LoadAgentConfig(root string) (AgentConfig, error) {
	cfg := AgentConfig{
		Default:   "none",
		Templates: map[string]string{},
	}
	path := filepath.Join(root, ".kai", "config")
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch {
		case key == "agent":
			cfg.Default = val
		case key == "columns":
			for _, c := range strings.Split(val, ",") {
				c = strings.TrimSpace(c)
				if c != "" {
					cfg.Columns = append(cfg.Columns, c)
				}
			}
		case strings.HasPrefix(key, "agent."):
			name := strings.TrimPrefix(key, "agent.")
			if name != "" {
				cfg.Templates[name] = val
			}
		}
	}
	return cfg, nil
}

// AgentNames returns cycle order: none, named templates (sorted), custom.
func AgentNames(cfg AgentConfig) []string {
	names := []string{"none"}
	keys := make([]string, 0, len(cfg.Templates))
	for k := range cfg.Templates {
		keys = append(keys, k)
	}
	// stable-ish: cursor, claude, codex first if present, then rest alpha
	prefer := []string{"cursor", "claude", "codex"}
	seen := map[string]bool{}
	for _, p := range prefer {
		if _, ok := cfg.Templates[p]; ok {
			names = append(names, p)
			seen[p] = true
		}
	}
	for _, k := range sortedStrings(keys) {
		if !seen[k] {
			names = append(names, k)
		}
	}
	names = append(names, "custom")
	return names
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
