package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kai not found in this project.\n")
		fmt.Fprintf(os.Stderr, "Install and init with:  npx kaijou\n")
		fmt.Fprintf(os.Stderr, "Then run the UI with:     npx kaitui\n")
		os.Exit(1)
	}
	if !isExecFile(kaiBin(root)) {
		fmt.Fprintf(os.Stderr, "kai CLI missing next to this board.\n")
		fmt.Fprintf(os.Stderr, "Install with:  npx kaijou\n")
		os.Exit(1)
	}
	cfg, err := LoadAgentConfig(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load config: %v\n", err)
		fmt.Fprintf(os.Stderr, "hint: run  npx kaijou  to init .kai/\n")
		os.Exit(1)
	}
	board, err := LoadBoard(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load board: %v\n", err)
		os.Exit(1)
	}

	m := initialModel(root, cfg, board)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
