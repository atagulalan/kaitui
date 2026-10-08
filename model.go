package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type hitKind int

const (
	hitNone hitKind = iota
	hitCard
	hitResize
	hitBtnRefresh
	hitBtnQuit
	hitAILog
	hitCol
	hitModal
)

const (
	minColWidth   = 12
	cardBodyLines = 3 // id+icon / title / time
	aiLogMinH     = 1
	aiLogMaxH     = 4
	headerH       = 1
)

type hitTarget struct {
	kind   hitKind
	cardID string
	col    string
	index  int
	x, y   int
	w, h   int
}

type scrollDragKind int

const (
	scrollNone scrollDragKind = iota
	scrollCol
	scrollAI
	scrollModal
)

type model struct {
	root     string
	board    Board
	agentCfg AgentConfig
	selected map[string]bool
	input    textinput.Model
	width    int
	height   int
	status   string
	running  bool
	hits     []hitTarget
	scroll   map[string]int // column -> card offset

	colWidths []int
	resizing  bool
	resizeIdx int
	resizeX0  int
	resizeW0  int
	resizeN0  int

	aiLog    []aiEntry
	aiScroll int

	// detail modal
	detailOpen   bool
	detailID     string
	detailBody   string
	detailScroll int
	detailLoading bool

	// drag scroll / click-vs-drag
	scrollDrag    scrollDragKind
	scrollColName string
	scrollY0      int
	scrollOff0    int
	pressCardID   string
	pressCol      string // empty-column press (clear vs scroll)
	pressAIIdx    int    // ai log line index; -1 = none
	pressY        int
	pressMoved    bool
}

func initialModel(root string, cfg AgentConfig, board Board) *model {
	ti := textinput.New()
	ti.Placeholder = "/help · or ask agent…"
	ti.CharLimit = 2000
	ti.Width = 40
	ti.Prompt = ""
	ti.PromptStyle = lipgloss.NewStyle()

	m := &model{
		root:       root,
		board:      board,
		agentCfg:   cfg,
		selected:   map[string]bool{},
		input:      ti,
		scroll:     map[string]int{},
		pressAIIdx: -1,
		status:     "left: detail · right: select · AI line: full summary",
	}
	if m.agent() != "none" {
		ti.Focus()
		m.input = ti
	}
	if len(cfg.Columns) > 0 && len(board.Columns) == 0 {
		m.board.Columns = append([]string{}, cfg.Columns...)
		m.board.Cards = map[string][]Card{}
		for _, c := range cfg.Columns {
			m.board.Cards[c] = nil
		}
	} else if len(cfg.Columns) > 0 {
		seen := map[string]bool{}
		for _, c := range m.board.Columns {
			seen[c] = true
		}
		for _, c := range cfg.Columns {
			if !seen[c] {
				m.board.Columns = append(m.board.Columns, c)
				m.board.Cards[c] = nil
			}
		}
	}
	return m
}

func (m *model) agent() string {
	a := strings.TrimSpace(m.agentCfg.Default)
	if a == "" {
		return "none"
	}
	return a
}

func (m *model) projectTitle() string {
	base := filepath.Base(m.root)
	if base == "" || base == "." {
		return "kai"
	}
	return base
}

func (m *model) Init() tea.Cmd {
	return nil
}

func refreshBoard(root string) tea.Cmd {
	return func() tea.Msg {
		b, err := LoadBoard(root)
		return boardLoadedMsg{board: b, err: err}
	}
}

type boardLoadedMsg struct {
	board Board
	err   error
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = max(10, msg.Width-lipgloss.Width(m.agent())-4)
		m.ensureColWidths(true)
		return m, nil

	case boardLoadedMsg:
		if msg.err != nil {
			m.status = "refresh error: " + msg.err.Error()
			return m, nil
		}
		m.board = msg.board
		if len(m.agentCfg.Columns) > 0 {
			for _, c := range m.agentCfg.Columns {
				if _, ok := m.board.Cards[c]; !ok {
					m.board.Columns = append(m.board.Columns, c)
					m.board.Cards[c] = nil
				}
			}
		}
		m.ensureColWidths(false)
		m.status = "board refreshed"
		return m, nil

	case agentResultMsg:
		m.running = false
		if msg.err != nil {
			line := truncate(strings.TrimSpace(msg.out+" "+msg.err.Error()), max(40, m.width-2))
			detail := agentTranscript(msg.out)
			if detail == "" {
				detail = line
			}
			m.appendAI("error: "+line, detail)
			m.status = "agent error · .kai/agent.log"
		} else {
			reply := agentReply(msg.out)
			if reply == "" {
				reply = "(empty — see .kai/agent.log)"
			}
			detail := agentTranscript(msg.out)
			if detail == "" {
				detail = reply
			}
			m.appendAI(reply, detail)
			m.status = "agent done"
			m.input.SetValue("")
		}
		return m, refreshBoard(m.root)

	case detailLoadedMsg:
		m.detailLoading = false
		if msg.id != m.detailID {
			return m, nil
		}
		if msg.err != nil {
			m.detailBody = "error: " + msg.err.Error()
		} else {
			m.detailBody = msg.body
		}
		m.detailScroll = 0
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		if m.running {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.detailOpen {
			switch msg.String() {
			case "esc", "q":
				m.closeDetail()
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			case "up":
				m.detailScroll = max(0, m.detailScroll-1)
				return m, nil
			case "down":
				m.detailScroll++
				return m, nil
			}
			return m, nil
		}
		if m.resizing || m.scrollDrag != scrollNone {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.agent() == "none" {
				return m, nil
			}
			return m.handleEnter()
		}
		if m.agent() != "none" {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

type aiEntry struct {
	Summary string // one-line strip
	Detail  string // full transcript for modal
}

func (m *model) appendAI(summary, detail string) {
	summary = strings.TrimSpace(summary)
	detail = strings.TrimSpace(detail)
	if summary == "" && detail == "" {
		return
	}
	if summary == "" {
		summary = truncate(detail, 140)
	}
	if detail == "" {
		detail = summary
	}
	m.aiLog = append(m.aiLog, aiEntry{Summary: summary, Detail: detail})
	// keep scroll at bottom (show newest)
	vis := m.aiVisibleH()
	if len(m.aiLog) > vis {
		m.aiScroll = len(m.aiLog) - vis
	} else {
		m.aiScroll = 0
	}
}

func (m *model) aiVisibleH() int {
	h := aiLogMinH
	n := len(m.aiLog)
	if n > h {
		h = n
	}
	if h > aiLogMaxH {
		h = aiLogMaxH
	}
	return h
}

func (m *model) handleEnter() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	if cmd, ok := parseSlash(text); ok {
		m.input.SetValue("")
		return m.runSlash(cmd)
	}
	return m.submitAgent()
}

func (m *model) runSlash(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/refresh":
		m.status = "refreshing…"
		return m, refreshBoard(m.root)
	case "/quit", "/q":
		return m, tea.Quit
	case "/clear":
		m.clearSelection()
		return m, nil
	case "/help":
		m.status = "/refresh /quit /clear /help · enter asks agent"
		return m, nil
	default:
		m.status = "unknown " + cmd + " · /help"
		return m, nil
	}
}

func (m *model) columns() []string {
	cols := m.board.Columns
	if len(cols) == 0 {
		cols = m.agentCfg.Columns
	}
	return cols
}

func (m *model) ensureColWidths(forceEqual bool) {
	cols := m.columns()
	n := len(cols)
	if n == 0 || m.width <= 0 {
		return
	}
	avail := m.width
	if avail < n*minColWidth {
		avail = n * minColWidth
	}
	if forceEqual || len(m.colWidths) != n {
		base := avail / n
		if base < minColWidth {
			base = minColWidth
		}
		m.colWidths = make([]int, n)
		for i := range m.colWidths {
			m.colWidths[i] = base
		}
		sum := base * n
		m.colWidths[n-1] += avail - sum
		if m.colWidths[n-1] < minColWidth {
			m.colWidths[n-1] = minColWidth
		}
		return
	}
	sum := 0
	for _, w := range m.colWidths {
		sum += w
	}
	if sum <= 0 {
		m.ensureColWidths(true)
		return
	}
	if sum == avail {
		return
	}
	newW := make([]int, n)
	used := 0
	for i := 0; i < n-1; i++ {
		w := m.colWidths[i] * avail / sum
		if w < minColWidth {
			w = minColWidth
		}
		newW[i] = w
		used += w
	}
	newW[n-1] = avail - used
	if newW[n-1] < minColWidth {
		newW[n-1] = minColWidth
	}
	m.colWidths = newW
}

func (m *model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Wheel
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			delta := 1
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -1
			}
			return m.handleWheel(msg.X, msg.Y, delta)
		}
	}

	if m.detailOpen {
		return m.handleModalMouse(msg)
	}

	switch msg.Action {
	case tea.MouseActionRelease:
		if msg.Button == tea.MouseButtonLeft {
			if m.resizing {
				m.resizing = false
				m.status = "column resized"
				return m, nil
			}
			if m.scrollDrag != scrollNone {
				m.scrollDrag = scrollNone
				m.clearPress()
				return m, nil
			}
			// click (no drag) on AI summary → modal
			if m.pressAIIdx >= 0 && !m.pressMoved {
				idx := m.pressAIIdx
				m.clearPress()
				return m.openAIDetail(idx)
			}
			// click (no drag) on card → detail
			if m.pressCardID != "" && !m.pressMoved {
				id := m.pressCardID
				m.clearPress()
				return m.openDetail(id)
			}
			// click empty column / board → clear selection
			if !m.pressMoved && m.pressAIIdx < 0 && (m.pressCol != "" || m.pressCardID == "") {
				if len(m.selected) > 0 {
					m.clearSelection()
				}
			}
			m.clearPress()
		}
	case tea.MouseActionMotion:
		if m.resizing {
			m.applyResize(msg.X)
			return m, nil
		}
		if m.scrollDrag != scrollNone {
			m.applyScrollDrag(msg.Y)
			return m, nil
		}
		// promote AI press to AI scroll if dragged
		if m.pressAIIdx >= 0 && abs(msg.Y-m.pressY) >= 2 {
			m.pressMoved = true
			m.scrollDrag = scrollAI
			m.scrollY0 = m.pressY
			m.scrollOff0 = m.aiScroll
			m.pressAIIdx = -1
			m.applyScrollDrag(msg.Y)
			return m, nil
		}
		// promote press to column scroll if dragged
		if (m.pressCardID != "" || m.pressCol != "") && abs(msg.Y-m.pressY) >= 2 {
			m.pressMoved = true
			col := m.pressCol
			if col == "" {
				col = m.cardColumn(m.pressCardID)
			}
			if col != "" {
				m.scrollDrag = scrollCol
				m.scrollColName = col
				m.scrollY0 = m.pressY
				m.scrollOff0 = m.scroll[col]
				m.pressCardID = ""
				m.pressCol = ""
				m.applyScrollDrag(msg.Y)
			}
			return m, nil
		}
	case tea.MouseActionPress:
		left := msg.Button == tea.MouseButtonLeft
		right := msg.Button == tea.MouseButtonRight
		if !left && !right {
			return m, nil
		}

		if left {
			if h := m.hitAt(msg.X, msg.Y, hitBtnRefresh); h != nil {
				m.status = "refreshing…"
				return m, refreshBoard(m.root)
			}
			if h := m.hitAt(msg.X, msg.Y, hitBtnQuit); h != nil {
				return m, tea.Quit
			}
			if h := m.hitAt(msg.X, msg.Y, hitResize); h != nil {
				if h.index < 0 || h.index+1 >= len(m.colWidths) {
					return m, nil
				}
				m.resizing = true
				m.resizeIdx = h.index
				m.resizeX0 = msg.X
				m.resizeW0 = m.colWidths[h.index]
				m.resizeN0 = m.colWidths[h.index+1]
				m.status = "resizing…"
				return m, nil
			}
			if h := m.hitAt(msg.X, msg.Y, hitAILog); h != nil {
				m.pressAIIdx = h.index
				m.pressCardID = ""
				m.pressCol = ""
				m.pressY = msg.Y
				m.pressMoved = false
				return m, nil
			}
			if h := m.hitAt(msg.X, msg.Y, hitCard); h != nil {
				m.pressCardID = h.cardID
				m.pressCol = ""
				m.pressAIIdx = -1
				m.pressY = msg.Y
				m.pressMoved = false
				return m, nil
			}
			if h := m.hitAt(msg.X, msg.Y, hitCol); h != nil {
				m.pressCol = h.col
				m.pressCardID = ""
				m.pressAIIdx = -1
				m.pressY = msg.Y
				m.pressMoved = false
				return m, nil
			}
			m.clearPress()
			m.pressY = msg.Y
			return m, nil
		}

		if right {
			if h := m.hitAt(msg.X, msg.Y, hitCard); h != nil {
				m.toggleSelect(h.cardID)
				return m, nil
			}
		}
	}
	return m, nil
}

func (m *model) handleModalMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Action {
	case tea.MouseActionPress:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.detailScroll = max(0, m.detailScroll-1)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.detailScroll++
			return m, nil
		case tea.MouseButtonLeft:
			if h := m.hitAt(msg.X, msg.Y, hitModal); h != nil {
				m.scrollDrag = scrollModal
				m.scrollY0 = msg.Y
				m.scrollOff0 = m.detailScroll
				return m, nil
			}
			m.closeDetail()
			return m, nil
		}
	case tea.MouseActionMotion:
		if m.scrollDrag == scrollModal {
			m.applyScrollDrag(msg.Y)
			return m, nil
		}
	case tea.MouseActionRelease:
		if m.scrollDrag == scrollModal {
			m.scrollDrag = scrollNone
		}
	}
	return m, nil
}

func (m *model) handleWheel(x, y, delta int) (tea.Model, tea.Cmd) {
	if m.detailOpen {
		m.detailScroll = max(0, m.detailScroll+delta)
		return m, nil
	}
	if h := m.hitAt(x, y, hitAILog); h != nil {
		m.aiScroll = clampScroll(m.aiScroll+delta, len(m.aiLog), m.aiVisibleH())
		return m, nil
	}
	if h := m.hitAt(x, y, hitCol); h != nil {
		cards := len(m.board.Cards[h.col])
		m.scroll[h.col] = clampScroll(m.scroll[h.col]+delta, cards, m.colMaxVisible(h.col))
		return m, nil
	}
	if h := m.hitAt(x, y, hitCard); h != nil {
		col := m.cardColumn(h.cardID)
		if col != "" {
			cards := len(m.board.Cards[col])
			m.scroll[col] = clampScroll(m.scroll[col]+delta, cards, m.colMaxVisible(col))
		}
		return m, nil
	}
	return m, nil
}

func (m *model) applyScrollDrag(y int) {
	// drag down → content moves down → scroll offset decreases
	deltaLines := (m.scrollY0 - y) / 3 // ~3px per card/line step
	switch m.scrollDrag {
	case scrollCol:
		cards := len(m.board.Cards[m.scrollColName])
		m.scroll[m.scrollColName] = clampScroll(m.scrollOff0+deltaLines, cards, m.colMaxVisible(m.scrollColName))
	case scrollAI:
		m.aiScroll = clampScroll(m.scrollOff0+deltaLines, len(m.aiLog), m.aiVisibleH())
	case scrollModal:
		lines := strings.Split(m.detailBody, "\n")
		m.detailScroll = clampScroll(m.scrollOff0+deltaLines, len(lines), m.modalBodyH())
	}
}

func (m *model) colMaxVisible(col string) int {
	// approximate from last board height; conservative default
	boardH := m.height - headerH - m.footerLines()
	if boardH < 5 {
		boardH = 5
	}
	avail := boardH - 3
	if avail < cardBodyLines {
		return 1
	}
	return 1 + (avail-cardBodyLines)/(cardBodyLines+1)
}

func clampScroll(off, total, visible int) int {
	if visible < 1 {
		visible = 1
	}
	maxOff := total - visible
	if maxOff < 0 {
		maxOff = 0
	}
	if off < 0 {
		return 0
	}
	if off > maxOff {
		return maxOff
	}
	return off
}

func (m *model) cardColumn(id string) string {
	for _, col := range m.columns() {
		for _, c := range m.board.Cards[col] {
			if c.ID == id {
				return col
			}
		}
	}
	return ""
}

func (m *model) clearSelection() {
	m.selected = map[string]bool{}
	m.status = "selection cleared"
}

func (m *model) toggleSelect(id string) {
	if m.selected == nil {
		m.selected = map[string]bool{}
	}
	if m.selected[id] {
		delete(m.selected, id)
	} else {
		m.selected[id] = true
	}
	m.status = fmt.Sprintf("selected %d", len(m.selected))
}

func (m *model) clearPress() {
	m.pressCardID = ""
	m.pressCol = ""
	m.pressAIIdx = -1
	m.pressMoved = false
}

func (m *model) openDetail(id string) (tea.Model, tea.Cmd) {
	m.detailOpen = true
	m.detailID = id
	m.detailBody = "loading…"
	m.detailScroll = 0
	m.detailLoading = true
	m.status = "detail " + id
	return m, loadDetail(m.root, id)
}

func (m *model) openAIDetail(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.aiLog) {
		return m, nil
	}
	e := m.aiLog[idx]
	body := e.Detail
	if body == "" {
		body = e.Summary
	}
	m.detailOpen = true
	m.detailID = "agent"
	m.detailBody = body
	m.detailScroll = 0
	m.detailLoading = false
	m.status = "agent detail"
	return m, nil
}

func (m *model) closeDetail() {
	m.detailOpen = false
	m.detailID = ""
	m.detailBody = ""
	m.detailScroll = 0
	m.detailLoading = false
	m.scrollDrag = scrollNone
	m.status = "detail closed"
}

func (m *model) flatCardIDs() []string {
	var out []string
	for _, col := range m.columns() {
		for _, c := range m.board.Cards[col] {
			out = append(out, c.ID)
		}
	}
	return out
}

func (m *model) selectedList() []string {
	ids := m.flatCardIDs()
	var out []string
	for _, id := range ids {
		if m.selected[id] {
			out = append(out, id)
		}
	}
	for id := range m.selected {
		found := false
		for _, x := range out {
			if x == id {
				found = true
				break
			}
		}
		if !found {
			out = append(out, id)
		}
	}
	return out
}

func (m *model) selectedCSV() string {
	list := m.selectedList()
	if len(list) == 0 {
		return ""
	}
	return strings.Join(list, ",")
}

func (m *model) isSelected(id string) bool {
	return m.selected[id]
}

func (m *model) hitAt(x, y int, kind hitKind) *hitTarget {
	for i := range m.hits {
		h := &m.hits[i]
		if h.kind != kind {
			continue
		}
		if x >= h.x && x < h.x+h.w && y >= h.y && y < h.y+h.h {
			return h
		}
	}
	return nil
}

func (m *model) applyResize(x int) {
	i := m.resizeIdx
	if i < 0 || i+1 >= len(m.colWidths) {
		return
	}
	delta := x - m.resizeX0
	left := m.resizeW0 + delta
	right := m.resizeN0 - delta
	if left < minColWidth {
		right -= minColWidth - left
		left = minColWidth
	}
	if right < minColWidth {
		left -= minColWidth - right
		right = minColWidth
	}
	if left < minColWidth {
		left = minColWidth
	}
	if right < minColWidth {
		right = minColWidth
	}
	m.colWidths[i] = left
	m.colWidths[i+1] = right
}

func (m *model) submitAgent() (tea.Model, tea.Cmd) {
	if m.agent() == "none" {
		m.status = "agent=none — set agent in .kai/config"
		return m, nil
	}
	cmdStr, err := BuildAgentCommand(m.agent(), m.agentCfg.Templates, m.selectedCSV(), m.input.Value())
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.running = true
	m.status = "running " + m.agent() + "…"
	return m, runAgentCmd(m.root, cmdStr)
}

func (m *model) footerLines() int {
	// sep + ai log + optional input
	n := 1 + m.aiVisibleH()
	if m.agent() != "none" {
		n++
	}
	return n
}

func (m *model) View() string {
	if m.width == 0 {
		m.width = 80
		m.height = 24
	}
	m.hits = m.hits[:0]

	footerLines := m.footerLines()
	boardH := m.height - headerH - footerLines
	if boardH < 5 {
		boardH = 5
	}
	m.ensureColWidths(false)

	var b strings.Builder
	b.WriteString(m.renderHeader(0))
	b.WriteByte('\n')

	boardView := strings.TrimRight(m.renderBoard(headerH, boardH), "\n")
	b.WriteString(boardView)
	b.WriteByte('\n')

	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("─", max(10, m.width)))
	b.WriteString(sep)
	b.WriteByte('\n')

	aiY := lineCount(b.String())
	b.WriteString(m.renderAILog(aiY))

	if m.agent() != "none" {
		m.input.Width = max(10, m.width-lipgloss.Width(m.agent())-4)
		muted := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		b.WriteString(muted.Render(m.agent() + " › "))
		b.WriteString(m.input.View())
		b.WriteByte('\n')
	}

	view := strings.TrimRight(b.String(), "\n")
	if m.detailOpen {
		view = m.overlayModal(view)
	}
	return view
}

func (m *model) infoText() string {
	st := m.status
	if n := len(m.selected); n > 0 {
		st = fmt.Sprintf("sel %d · %s", n, st)
	}
	return st
}

func (m *model) renderHeader(row int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("219"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	if m.running {
		infoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	}

	btnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("246")).Padding(0, 1)
	refresh := btnStyle.Render("refresh")
	quit := btnStyle.Render("quit")
	btns := refresh + "  " + quit
	btnW := lipgloss.Width(btns)

	title := titleStyle.Render(m.projectTitle())
	titleW := lipgloss.Width(title)

	info := m.infoText()
	side := titleW + btnW + 2
	infoW := max(8, m.width-side)
	info = truncate(info, infoW)
	info = infoStyle.Render(info)
	infoW = lipgloss.Width(info)

	// layout: title | pad | info centered in remaining | pad | buttons
	remain := max(0, m.width-titleW-btnW)
	leftPad := max(0, (remain-infoW)/2)
	rightPad := max(0, remain-infoW-leftPad)

	// hitboxes for buttons at right
	quitW := lipgloss.Width(quit)
	refreshW := lipgloss.Width(refresh)
	btnX := titleW + leftPad + infoW + rightPad
	m.hits = append(m.hits,
		hitTarget{kind: hitBtnRefresh, x: btnX, y: row, w: refreshW, h: 1},
		hitTarget{kind: hitBtnQuit, x: btnX + refreshW + 2, y: row, w: quitW, h: 1},
	)

	return title + strings.Repeat(" ", leftPad) + info + strings.Repeat(" ", rightPad) + btns
}

func (m *model) renderAILog(startY int) string {
	h := m.aiVisibleH()
	lines := m.aiLog
	off := clampScroll(m.aiScroll, len(lines), h)
	m.aiScroll = off

	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color("159"))

	var out []string
	for i := 0; i < h; i++ {
		idx := off + i
		switch {
		case len(lines) == 0 && i == 0:
			out = append(out, muted.Render(truncate("(no agent output)", m.width)))
		case idx < len(lines):
			out = append(out, bright.Render(truncate(lines[idx].Summary, m.width)))
			m.hits = append(m.hits, hitTarget{
				kind:  hitAILog,
				index: idx,
				x:     0,
				y:     startY + i,
				w:     m.width,
				h:     1,
			})
		default:
			out = append(out, strings.Repeat(" ", max(1, m.width)))
		}
	}

	// whole strip still scrollable via wheel even on empty rows
	if len(lines) == 0 {
		m.hits = append(m.hits, hitTarget{
			kind:  hitAILog,
			index: -1,
			x:     0,
			y:     startY,
			w:     m.width,
			h:     h,
		})
	}
	return strings.Join(out, "\n") + "\n"
}

func (m *model) modalBodyH() int {
	h := m.height - 6
	if h < 5 {
		h = 5
	}
	return h
}

func (m *model) overlayModal(base string) string {
	innerW := max(40, m.width*3/4)
	if innerW > m.width-4 {
		innerW = max(20, m.width-4)
	}
	textW := innerW - 2

	bodyLines := wrapText(m.detailBody, textW)
	bodyH := m.modalBodyH()
	off := clampScroll(m.detailScroll, len(bodyLines), bodyH)
	m.detailScroll = off

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("219")).Render(truncate(m.detailID, textW))
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("esc/q close · wheel scroll")

	var inner []string
	inner = append(inner, title)
	for i := 0; i < bodyH; i++ {
		idx := off + i
		if idx < len(bodyLines) {
			inner = append(inner, bodyLines[idx])
		} else {
			inner = append(inner, "")
		}
	}
	inner = append(inner, hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("212")).
		Width(innerW).
		Padding(0, 1).
		Render(strings.Join(inner, "\n"))

	boxW := lipgloss.Width(box)
	boxH := lipgloss.Height(box)
	x0 := max(0, (m.width-boxW)/2)
	y0 := max(1, (m.height-boxH)/2)

	m.hits = append(m.hits, hitTarget{
		kind: hitModal,
		x:    x0,
		y:    y0,
		w:    boxW,
		h:    boxH,
	})

	baseLines := strings.Split(base, "\n")
	// pad base to height
	for len(baseLines) < m.height {
		baseLines = append(baseLines, strings.Repeat(" ", m.width))
	}
	boxRows := strings.Split(box, "\n")
	for i, row := range boxRows {
		y := y0 + i
		if y < 0 || y >= len(baseLines) {
			continue
		}
		baseLines[y] = overlayLine(baseLines[y], row, x0, m.width)
	}
	if len(baseLines) > m.height {
		baseLines = baseLines[:m.height]
	}
	return strings.Join(baseLines, "\n")
}

func overlayLine(base, over string, x0, width int) string {
	b := padRight(stripAnsi(base), width)
	o := stripAnsi(over)
	br := []rune(b)
	or := []rune(o)
	for i := 0; i < len(or); i++ {
		pos := x0 + i
		if pos >= 0 && pos < len(br) {
			br[pos] = or[i]
		}
	}
	// keep modal border color via wrapping — plain overlay is fine for hit math
	return string(br)
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func statusIcon(status string) (glyph string, color lipgloss.Color) {
	switch status {
	case "ready":
		return "●", "114"
	case "blocked":
		return "⊘", "203"
	default:
		return "○", "245"
	}
}

func (m *model) renderCardLines(c Card, textW int) []string {
	icon, col := statusIcon(c.Status)
	iconStyled := lipgloss.NewStyle().Foreground(col).Render(icon)
	idStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	when := c.Created
	if when == "" {
		when = "—"
	}
	header := lipgloss.JoinHorizontal(
		lipgloss.Center,
		lipgloss.NewStyle().Width(max(1, textW-2)).Render(idStyle.Render(truncate(c.ID, max(1, textW-2)))),
		lipgloss.NewStyle().Width(2).Align(lipgloss.Right).Render(iconStyled),
	)
	lines := []string{
		header,
		titleStyle.Render(truncate(c.Title, textW)),
		timeStyle.Render(truncate(when, textW)),
	}
	if m.isSelected(c.ID) {
		sel := lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("219"))
		for i, ln := range lines {
			lines[i] = sel.Render(padRight(stripAnsi(ln), textW))
		}
	}
	return lines
}

func stripAnsi(s string) string {
	var b strings.Builder
	in := false
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			in = true
			continue
		}
		if in {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				in = false
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func (m *model) renderBoard(startY, height int) string {
	cols := m.columns()
	if len(cols) == 0 {
		return "(no columns)\n"
	}
	if len(m.colWidths) != len(cols) {
		m.ensureColWidths(true)
	}

	parts := make([]string, 0, len(cols))
	x0 := 0
	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	for ci, col := range cols {
		colW := m.colWidths[ci]
		if colW < minColWidth {
			colW = minColWidth
		}
		innerW := max(8, colW-2)
		textW := max(4, innerW-2)

		title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render(col)

		cards := m.board.Cards[col]
		avail := height - 3
		maxCards := 1
		if avail >= cardBodyLines {
			maxCards = 1 + (avail-cardBodyLines)/(cardBodyLines+1)
		}
		off := clampScroll(m.scroll[col], len(cards), maxCards)
		m.scroll[col] = off

		var inner []string
		inner = append(inner, title)
		yCursor := startY + 2

		m.hits = append(m.hits, hitTarget{
			kind: hitCol,
			col:  col,
			x:    x0,
			y:    startY,
			w:    colW,
			h:    height,
		})

		if len(cards) == 0 {
			inner = append(inner, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(empty)"))
		}

		shown := 0
		for i := off; i < len(cards) && shown < maxCards; i++ {
			if shown > 0 {
				inner = append(inner, divider.Render(strings.Repeat("─", textW)))
				yCursor++
			}
			c := cards[i]
			lines := m.renderCardLines(c, textW)
			inner = append(inner, lines...)
			m.hits = append(m.hits, hitTarget{
				kind:   hitCard,
				cardID: c.ID,
				col:    col,
				x:      x0 + 1,
				y:      yCursor,
				w:      max(1, colW-2),
				h:      cardBodyLines,
			})
			yCursor += cardBodyLines
			shown++
		}

		colBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Width(innerW).
			Height(height - 2).
			Align(lipgloss.Left, lipgloss.Top).
			Padding(0, 1).
			Render(strings.Join(inner, "\n"))

		colView := lipgloss.NewStyle().Width(colW).Height(height).Align(lipgloss.Left, lipgloss.Top).Render(colBox)
		parts = append(parts, colView)

		boxW := lipgloss.Width(colView)
		if boxW <= 0 {
			boxW = colW
		}
		if ci < len(cols)-1 {
			m.hits = append(m.hits, hitTarget{
				kind:  hitResize,
				index: ci,
				x:     x0 + boxW - 2,
				y:     startY,
				w:     3,
				h:     height,
			})
		}
		x0 += boxW
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...) + "\n"
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// wrapText wraps s to width w (runes), preserving existing newlines.
func wrapText(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		r := []rune(para)
		for len(r) > w {
			out = append(out, string(r[:w]))
			r = r[w:]
		}
		out = append(out, string(r))
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return truncate(s, n)
	}
	return s + strings.Repeat(" ", n-w)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
