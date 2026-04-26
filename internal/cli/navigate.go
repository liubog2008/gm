package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/liubog2008/gm/internal/config"
	"github.com/liubog2008/gm/internal/repo"
)

type navigateOptions struct {
	filter       string
	outputAll    bool
	onlyRepo     bool
	onlyWorktree bool
}

type navRow struct {
	entry      repo.Entry
	label      string
	branch     string
	depth      int
	selectable bool
}

type entryGroup struct {
	repo      repo.Entry
	worktrees []repo.Entry
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230"))

	repoStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("110"))

	repoMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("109"))

	worktreeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	branchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("179"))

	pathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("109"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("62")).
			Bold(true)
)

func runNavigate(ctx context.Context, manager *repo.Manager, stdout io.Writer, configPath string, opts navigateOptions) error {
	if opts.onlyRepo && opts.onlyWorktree {
		return fmt.Errorf("%w: -r and -w cannot be used together", errUsage)
	}

	repos, err := manager.List(ctx)
	if err != nil {
		return err
	}

	recentPath, err := config.StatePath(configPath, "recent.json")
	if err != nil {
		return err
	}
	recent, err := loadRecent(recentPath)
	if err != nil {
		return err
	}

	entries := repo.FilterEntries(repo.BuildEntries(repos), repo.EntryFilters{
		Query:         opts.filter,
		OnlyRepo:      opts.onlyRepo,
		OnlyWorktree:  opts.onlyWorktree,
		RecentWeights: recent,
	})
	if opts.outputAll {
		return printEntries(stdout, entries, recent)
	}

	selected, err := selectEntry(entries, recent, opts)
	if err != nil {
		return err
	}

	recent = markRecent(selected.Path, recent)
	if err := saveRecent(recentPath, recent); err != nil {
		return err
	}
	return printDir(stdout, selected.Path)
}

func selectEntry(entries []repo.Entry, recent map[string]int64, opts navigateOptions) (repo.Entry, error) {
	if len(entries) == 0 {
		return repo.Entry{}, fmt.Errorf("no match for %q", strings.TrimSpace(opts.filter))
	}
	if len(entries) == 1 {
		return entries[0], nil
	}
	return runEntryTUI(entries, recent, opts)
}

func printEntries(stdout io.Writer, entries []repo.Entry, recent map[string]int64) error {
	for _, item := range entries {
		branch := item.Branch
		if branch == "" {
			branch = "-"
		}
		legacy := "false"
		if item.Legacy {
			legacy = "true"
		}
		if _, err := fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
			item.Kind,
			item.RepoKey,
			item.Name,
			branch,
			item.Path,
			item.RemoteURL,
			legacy,
			recent[item.Path],
		); err != nil {
			return err
		}
	}
	return nil
}

func runEntryTUI(initial []repo.Entry, recent map[string]int64, opts navigateOptions) (repo.Entry, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return repo.Entry{}, fmt.Errorf("multiple matches; use -f to filter or run in a terminal")
	}
	defer tty.Close()

	model := newNavigatorModel(initial, recent, opts)
	program := tea.NewProgram(model, tea.WithInput(tty), tea.WithOutput(tty), tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return repo.Entry{}, err
	}

	done := finalModel.(navigatorModel)
	if done.cancelled {
		return repo.Entry{}, fmt.Errorf("cancelled")
	}
	if done.selected.Path == "" {
		return repo.Entry{}, fmt.Errorf("no selection")
	}
	return done.selected, nil
}

type navigatorModel struct {
	all       []repo.Entry
	recent    map[string]int64
	opts      navigateOptions
	cwd       string
	input     textinput.Model
	rows      []navRow
	cursor    int
	offset    int
	width     int
	height    int
	selected  repo.Entry
	cancelled bool
}

func newNavigatorModel(entries []repo.Entry, recent map[string]int64, opts navigateOptions) navigatorModel {
	input := textinput.New()
	input.Prompt = "  "
	input.SetValue(opts.filter)
	input.Focus()
	input.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	input.Placeholder = "type to filter repos and worktrees"

	cwd, _ := os.Getwd()

	m := navigatorModel{
		all:    entries,
		recent: recent,
		opts:   opts,
		cwd:    cwd,
		input:  input,
		height: 20,
		cursor: -1,
	}
	m.refresh()
	return m
}

func (m navigatorModel) Init() tea.Cmd {
	return nil
}

func (m navigatorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureCursorVisible()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
				return m, nil
			}
			m.selected = m.rows[m.cursor].entry
			return m, tea.Quit
		case "up":
			m.moveCursor(-1)
			return m, nil
		case "down":
			m.moveCursor(1)
			return m, nil
		case "backspace":
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.refresh()
			return m, cmd
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.refresh()
	return m, cmd
}

func (m *navigatorModel) refresh() {
	query := strings.TrimSpace(m.input.Value())
	filters := repo.EntryFilters{
		Query:         query,
		OnlyRepo:      m.opts.onlyRepo,
		OnlyWorktree:  m.opts.onlyWorktree,
		RecentWeights: m.recent,
	}
	ranked := repo.FilterEntries(m.all, filters)
	prevPath := ""
	if len(m.rows) > 0 && m.cursor >= 0 && m.cursor < len(m.rows) {
		prevPath = m.rows[m.cursor].entry.Path
	}
	expandedRepoKey := ""
	if prevPath != "" {
		expandedRepoKey = repoKeyByPath(ranked, prevPath)
	}
	if expandedRepoKey == "" && query == "" {
		expandedRepoKey = m.currentRepoKey()
	}
	if expandedRepoKey == "" && len(ranked) > 0 {
		expandedRepoKey = ranked[0].RepoKey
	}
	m.rows = buildRows(m.all, filters, ranked, expandedRepoKey)
	if len(m.rows) == 0 {
		m.cursor = -1
		m.offset = 0
		return
	}

	if query == "" {
		m.cursor = m.currentRepoCursor()
		if m.cursor >= 0 {
			m.ensureCursorVisible()
			return
		}
		m.offset = 0
		return
	}

	for i, row := range m.rows {
		if row.selectable && row.entry.Path == prevPath {
			m.cursor = i
			m.ensureCursorVisible()
			return
		}
	}
	if len(ranked) > 0 {
		m.cursor = cursorByPath(m.rows, ranked[0].Path)
	}
	if m.cursor < 0 {
		m.cursor = firstSelectable(m.rows)
	}
	m.ensureCursorVisible()
}

func (m navigatorModel) View() string {
	start, end := m.visibleWindow()
	listLines := make([]string, 0, max(end-start, 1))
	if len(m.rows) == 0 {
		listLines = append(listLines, statusStyle.Render("No matches"))
	} else {
		for i := start; i < end; i++ {
			listLines = append(listLines, m.renderRow(i, m.rows[i]))
		}
	}

	footer := []string{
		titleStyle.Render(m.titleLine()),
		m.input.View(),
		statusStyle.Render("Enter select  •  ↑/↓ move  •  Esc cancel"),
	}

	bodyHeight := m.height - len(footer)
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	lines := make([]string, 0, bodyHeight+len(footer))
	for len(lines)+len(listLines) < bodyHeight {
		lines = append(lines, "")
	}
	lines = append(lines, listLines...)
	lines = append(lines, footer...)
	return strings.Join(lines, "\n")
}

func (m navigatorModel) visibleWindow() (int, int) {
	page := m.height - 5
	if page < 3 {
		page = 3
	}
	end := m.offset + page
	if end > len(m.rows) {
		end = len(m.rows)
	}
	return m.offset, end
}

func (m *navigatorModel) ensureCursorVisible() {
	page := m.height - 5
	if page < 3 {
		page = 3
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+page {
		m.offset = m.cursor - page + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
	maxOffset := len(m.rows) - page
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
}

func (m navigatorModel) renderRow(idx int, row navRow) string {
	line := ""
	if row.depth == 0 {
		meta := "[legacy repo]"
		if !row.entry.Legacy {
			meta = "[.bare]"
		}
		line = fmt.Sprintf("%s %s %s",
			"󰉋",
			repoStyle.Render(row.entry.RepoKey),
			repoMetaStyle.Render(meta),
		)
	} else {
		branch := row.branch
		if branch == "" {
			branch = "-"
		}
		line = fmt.Sprintf("%s%s %s  %s  %s",
			strings.Repeat("  ", row.depth),
			"󰆍",
			worktreeStyle.Render(row.label),
			branchStyle.Render(" "+branch),
			pathStyle.Render(row.entry.Path),
		)
	}
	if idx == m.cursor {
		return selectedStyle.Render("▌ " + line)
	}
	return "  " + line
}

func (m navigatorModel) titleLine() string {
	parts := []string{"gm"}
	if m.opts.onlyRepo {
		parts = append(parts, "repo")
	}
	if m.opts.onlyWorktree {
		parts = append(parts, "worktree")
	}
	if len(m.rows) > 0 && m.cursor >= 0 && m.cursor < len(m.rows) {
		parts = append(parts, fmt.Sprintf("%d/%d", m.cursor+1, len(m.rows)))
	}
	return strings.Join(parts, "  •  ")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *navigatorModel) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	next := m.cursor
	if next < 0 {
		if delta > 0 {
			next = -1
		} else {
			next = len(m.rows)
		}
	}
	for {
		next += delta
		if next < 0 || next >= len(m.rows) {
			return
		}
		if m.rows[next].selectable {
			m.cursor = next
			m.ensureCursorVisible()
			return
		}
	}
}

func (m navigatorModel) currentRepoCursor() int {
	if m.cwd == "" {
		return -1
	}
	bestIdx := -1
	bestLen := -1
	cwd := filepathClean(m.cwd)
	for i, row := range m.rows {
		if !row.selectable || row.entry.Kind != repo.EntryKindRepo {
			continue
		}
		root := repoRootPath(row.entry)
		if root == "" {
			continue
		}
		root = filepathClean(root)
		if cwd == root || strings.HasPrefix(cwd, root+string(os.PathSeparator)) {
			if len(root) > bestLen {
				bestLen = len(root)
				bestIdx = i
			}
		}
	}
	return bestIdx
}

func repoRootPath(entry repo.Entry) string {
	if entry.Legacy {
		return entry.Path
	}
	if entry.Kind == repo.EntryKindRepo {
		return strings.TrimSuffix(entry.Path, string(os.PathSeparator)+".bare")
	}
	return ""
}

func filepathClean(path string) string {
	return strings.TrimRight(path, string(os.PathSeparator))
}

func buildRows(entries []repo.Entry, filters repo.EntryFilters, ranked []repo.Entry, expandedRepoKey string) []navRow {
	groups := groupEntries(entries)
	order := groupOrder(groups, ranked)
	query := strings.TrimSpace(strings.Trim(filters.Query, "/"))
	rows := make([]navRow, 0, len(entries))
	bestPath := ""
	bestRepoKey := ""
	if len(ranked) > 0 {
		bestPath = ranked[0].Path
		bestRepoKey = ranked[0].RepoKey
	}

	for _, key := range order {
		group := groups[key]
		repoMatch := query == "" || matchEntryForGroup(query, group.repo)
		worktrees := group.worktrees
		if query != "" && !repoMatch {
			filtered := make([]repo.Entry, 0, len(worktrees))
			for _, wt := range worktrees {
				if matchEntryForGroup(query, wt) {
					filtered = append(filtered, wt)
				}
			}
			worktrees = filtered
		}
		if query != "" && !repoMatch && len(worktrees) == 0 {
			continue
		}

		isExpanded := key == expandedRepoKey
		if !filters.OnlyRepo {
			worktrees = repo.FilterEntries(worktrees, repo.EntryFilters{
				Query:         query,
				OnlyWorktree:  true,
				RecentWeights: filters.RecentWeights,
			})
			if query == "" || repoMatch {
				worktrees = repo.FilterEntries(group.worktrees, repo.EntryFilters{
					Query:         "",
					OnlyWorktree:  true,
					RecentWeights: filters.RecentWeights,
				})
			}
			if isExpanded {
				worktrees = movePathToEnd(worktrees, bestPath)
				for _, wt := range worktrees {
					rows = append(rows, navRow{
						entry:      wt,
						label:      wt.Name,
						branch:     wt.Branch,
						depth:      1,
						selectable: true,
					})
				}
			}
		}
		if !filters.OnlyWorktree {
			label := group.repo.RepoKey
			if group.repo.Legacy {
				label += " [legacy repo]"
			} else {
				label += " [.bare]"
			}
			rows = append(rows, navRow{
				entry:      group.repo,
				label:      label,
				selectable: true,
			})
		}
		if key == bestRepoKey && filters.OnlyRepo {
			_ = bestRepoKey
		}
	}
	return rows
}

func groupEntries(entries []repo.Entry) map[string]entryGroup {
	groups := make(map[string]entryGroup)
	for _, entry := range entries {
		group := groups[entry.RepoKey]
		if entry.Kind == repo.EntryKindRepo {
			group.repo = entry
		} else {
			group.worktrees = append(group.worktrees, entry)
		}
		groups[entry.RepoKey] = group
	}
	return groups
}

func groupOrder(groups map[string]entryGroup, ranked []repo.Entry) []string {
	order := make([]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, entry := range ranked {
		if _, ok := seen[entry.RepoKey]; ok {
			continue
		}
		seen[entry.RepoKey] = struct{}{}
		order = append(order, entry.RepoKey)
	}
	for key := range groups {
		if _, ok := seen[key]; ok {
			continue
		}
		order = append(order, key)
	}
	tail := order[len(seen):]
	sort.Strings(tail)
	if len(ranked) > 0 {
		bestRepoKey := ranked[0].RepoKey
		order = moveRepoKeyToEnd(order, bestRepoKey)
	}
	return order
}

func matchEntryForGroup(query string, entry repo.Entry) bool {
	filtered := repo.FilterEntries([]repo.Entry{entry}, repo.EntryFilters{Query: query})
	return len(filtered) > 0
}

func firstSelectable(rows []navRow) int {
	for i, row := range rows {
		if row.selectable {
			return i
		}
	}
	return -1
}

func cursorByPath(rows []navRow, path string) int {
	for i, row := range rows {
		if row.selectable && row.entry.Path == path {
			return i
		}
	}
	return -1
}

func repoKeyByPath(entries []repo.Entry, path string) string {
	for _, entry := range entries {
		if entry.Path == path {
			return entry.RepoKey
		}
	}
	return ""
}

func (m navigatorModel) currentRepoKey() string {
	idx := m.currentRepoCursor()
	if idx < 0 || idx >= len(m.rows) {
		return ""
	}
	return m.rows[idx].entry.RepoKey
}

func moveRepoKeyToEnd(order []string, repoKey string) []string {
	if repoKey == "" {
		return order
	}
	out := make([]string, 0, len(order))
	found := false
	for _, key := range order {
		if key == repoKey {
			found = true
			continue
		}
		out = append(out, key)
	}
	if found {
		out = append(out, repoKey)
	}
	return out
}

func movePathToEnd(entries []repo.Entry, path string) []repo.Entry {
	if path == "" {
		return entries
	}
	out := make([]repo.Entry, 0, len(entries))
	var target *repo.Entry
	for i := range entries {
		if entries[i].Path == path {
			copy := entries[i]
			target = &copy
			continue
		}
		out = append(out, entries[i])
	}
	if target != nil {
		out = append(out, *target)
	}
	return out
}
