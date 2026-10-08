package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

const kaiAgentCheatSheet = `Use ./kai only — never scan/read .kai/ files directly.
Do not run ./kai help or --help; commands below are enough.
Commands:
  list [--all] [--column C] [--details]
  ready [--all] [--details]
  show <id>
  add --author ai --title "..." [--column todo] [--priority N] [--body "..."] [--depends id1,id2]
  depend <id> <dep-id> | undepend <id> <dep-id>
  move <id> <column> | priority <id> <n> | done <id>
  board | workflow
Prefer move/done/add over exploring. Author for new cards: ai.`

func buildAgentPrompt(selectedCSV, userText string) string {
	id := selectedCSV
	if id == "" {
		id = "(none)"
	}
	label := "Selected card"
	if strings.Contains(id, ",") {
		label = "Selected cards"
	}
	return fmt.Sprintf(
		"%s\n%s: %s\nUser request: %s\nAfter tools, end with one summary sentence that answers the user (max 140 characters). Do not narrate progress.",
		kaiAgentCheatSheet, label, id, userText,
	)
}

// agentReply extracts the model's final text from noisy CLI output.
func agentReply(out string) string {
	lines := strings.Split(out, "\n")
	// Codex may emit several "codex" blocks; take the last real reply (not tool narration).
	for i := len(lines) - 2; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "codex" {
			continue
		}
		if reply := strings.TrimSpace(lines[i+1]); reply != "" && !agentMetaLine(reply) {
			return reply
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || agentMetaLine(line) {
			continue
		}
		return line
	}
	return ""
}

// agentTranscript keeps tool steps + replies; drops banner, user echo, and tokens footer.
func agentTranscript(out string) string {
	lines := strings.Split(out, "\n")
	var kept []string
	started := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "tokens used" {
			break
		}
		if !started {
			if t == "codex" || t == "exec" {
				started = true
			} else {
				continue
			}
		}
		switch {
		case t == "codex":
			if len(kept) > 0 {
				kept = append(kept, "")
			}
			kept = append(kept, "›")
		case t == "exec":
			if len(kept) > 0 {
				kept = append(kept, "")
			}
			kept = append(kept, "$")
		case t == "user", strings.HasPrefix(t, "hook:"):
			continue
		default:
			kept = append(kept, line)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func agentMetaLine(line string) bool {
	switch {
	case line == "codex", line == "user", line == "tokens used":
		return true
	case strings.HasPrefix(line, "OpenAI Codex"),
		strings.HasPrefix(line, "Reading additional"),
		strings.HasPrefix(line, "workdir:"),
		strings.HasPrefix(line, "model:"),
		strings.HasPrefix(line, "provider:"),
		strings.HasPrefix(line, "approval:"),
		strings.HasPrefix(line, "sandbox:"),
		strings.HasPrefix(line, "reasoning"),
		strings.HasPrefix(line, "session id:"),
		strings.HasPrefix(line, "hook:"),
		strings.HasPrefix(line, "-----"),
		strings.HasPrefix(line, "You are editing this repo"),
		strings.HasPrefix(line, "Use ./kai only"),
		strings.HasPrefix(line, "Selected card"),
		strings.HasPrefix(line, "User request:"),
		strings.HasPrefix(line, "Final reply:"),
		strings.HasPrefix(line, "After tools,"):
		return true
	case len(line) > 0 && line[0] >= '0' && line[0] <= '9':
		// token count lines like "3,197"
		for _, r := range line {
			if r != ',' && (r < '0' || r > '9') {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// BuildAgentCommand returns the shell command to run for the selected agent.
// custom: userText is executed as-is via sh -c.
// selectedCSV may be one id or comma-separated ids (also substituted for {id}).
func BuildAgentCommand(agent string, templates map[string]string, selectedCSV, userText string) (string, error) {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		return "", fmt.Errorf("empty prompt")
	}
	if agent == "custom" {
		return userText, nil
	}
	tmpl, ok := templates[agent]
	if !ok || tmpl == "" {
		return "", fmt.Errorf("no template for agent %q", agent)
	}
	prompt := buildAgentPrompt(selectedCSV, userText)
	cmd := tmpl
	cmd = strings.ReplaceAll(cmd, "{prompt}", shellQuote(prompt))
	cmd = strings.ReplaceAll(cmd, "{id}", shellQuote(selectedCSV))
	return cmd, nil
}

type agentResultMsg struct {
	out     string
	err     error
	logPath string
}

func agentLogPath(root string) string {
	return filepath.Join(root, ".kai", "agent.log")
}

func appendAgentLog(path, body string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(body)
	return err
}

func runAgentCmd(root, shellCmd string) tea.Cmd {
	return func() tea.Msg {
		logPath := agentLogPath(root)
		started := time.Now()
		_ = appendAgentLog(logPath, fmt.Sprintf(
			"\n===== %s =====\ncwd: %s\ncmd: %s\n",
			started.Format(time.RFC3339), root, shellCmd,
		))

		c := exec.Command("sh", "-c", shellCmd)
		c.Dir = root
		devNull, errOpen := os.Open(os.DevNull)
		if errOpen == nil {
			c.Stdin = devNull
			defer devNull.Close()
		}
		out, err := c.CombinedOutput()
		elapsed := time.Since(started).Round(time.Millisecond)

		exit := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				exit = ee.ExitCode()
			} else {
				exit = -1
			}
		}
		_ = appendAgentLog(logPath, fmt.Sprintf(
			"exit: %d\nelapsed: %s\n--- stdout+stderr ---\n%s\n===== end =====\n",
			exit, elapsed, string(out),
		))

		return agentResultMsg{out: string(out), err: err, logPath: logPath}
	}
}
