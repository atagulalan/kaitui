package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type detailLoadedMsg struct {
	id   string
	body string
	err  error
}

func loadDetail(root, id string) tea.Cmd {
	return func() tea.Msg {
		out, err := runKai(root, "show", id)
		return detailLoadedMsg{id: id, body: strings.TrimRight(out, "\n"), err: err}
	}
}
