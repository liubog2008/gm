package cli

import (
	"testing"

	"github.com/liubog2008/gm/internal/repo"
)

func TestEnsureCursorVisibleClampsOffsetWhenListShrinks(t *testing.T) {
	m := navigatorModel{
		rows: []navRow{
			{selectable: true},
			{selectable: true},
			{selectable: true},
			{selectable: true},
		},
		cursor: 3,
		offset: 3,
		height: 20,
	}

	m.ensureCursorVisible()

	if m.offset != 0 {
		t.Fatalf("offset = %d, want 0", m.offset)
	}
}

func TestEnsureCursorVisibleKeepsCursorInViewForLongLists(t *testing.T) {
	rows := make([]navRow, 20)
	for i := range rows {
		rows[i].selectable = true
	}
	m := navigatorModel{
		rows:   rows,
		cursor: 12,
		offset: 0,
		height: 10,
	}

	m.ensureCursorVisible()

	if m.offset != 8 {
		t.Fatalf("offset = %d, want 8", m.offset)
	}
}

func TestNavigatorDefaultsToMatchingWorktreeForSplitQuery(t *testing.T) {
	entries := []repo.Entry{
		{
			Kind:     repo.EntryKindRepo,
			RepoKey:  "github.com/acme/platform-gm",
			RepoPath: "acme/platform-gm",
			Label:    "github.com/acme/platform-gm/.bare",
			Path:     "/repo/.bare",
		},
		{
			Kind:     repo.EntryKindWorktree,
			RepoKey:  "github.com/acme/platform-gm",
			RepoPath: "acme/platform-gm",
			Name:     "main",
			Label:    "github.com/acme/platform-gm:main",
			Path:     "/repo/main",
		},
		{
			Kind:     repo.EntryKindWorktree,
			RepoKey:  "github.com/acme/platform-gm",
			RepoPath: "acme/platform-gm",
			Name:     "fix-render",
			Label:    "github.com/acme/platform-gm:fix-render",
			Path:     "/repo/fix-render",
		},
	}

	m := newNavigatorModel(entries, nil, navigateOptions{filter: ":fix"})

	if m.cursor < 0 || m.cursor >= len(m.rows) {
		t.Fatalf("cursor = %d, rows = %d", m.cursor, len(m.rows))
	}
	if got := m.rows[m.cursor].entry.Kind; got != repo.EntryKindWorktree {
		t.Fatalf("selected kind = %q, want %q", got, repo.EntryKindWorktree)
	}
	if got := m.rows[m.cursor].entry.Path; got != "/repo/fix-render" {
		t.Fatalf("selected path = %q, want %q", got, "/repo/fix-render")
	}
}

func TestNavigatorTypingSplitQueryMovesCursorFromRepoToMatchingWorktree(t *testing.T) {
	entries := []repo.Entry{
		{
			Kind:     repo.EntryKindRepo,
			RepoKey:  "github.com/acme/platform-gm",
			RepoPath: "acme/platform-gm",
			Label:    "github.com/acme/platform-gm/.bare",
			Path:     "/repo/.bare",
		},
		{
			Kind:     repo.EntryKindWorktree,
			RepoKey:  "github.com/acme/platform-gm",
			RepoPath: "acme/platform-gm",
			Name:     "main",
			Label:    "github.com/acme/platform-gm:main",
			Path:     "/repo/main",
		},
		{
			Kind:     repo.EntryKindWorktree,
			RepoKey:  "github.com/acme/platform-gm",
			RepoPath: "acme/platform-gm",
			Name:     "fix-render",
			Label:    "github.com/acme/platform-gm:fix-render",
			Path:     "/repo/fix-render",
		},
	}

	m := newNavigatorModel(entries, nil, navigateOptions{})
	m.cursor = cursorByPath(m.rows, "/repo/.bare")
	m.input.SetValue(":fix")
	m.refresh()

	if m.cursor < 0 || m.cursor >= len(m.rows) {
		t.Fatalf("cursor = %d, rows = %d", m.cursor, len(m.rows))
	}
	if got := m.rows[m.cursor].entry.Kind; got != repo.EntryKindWorktree {
		t.Fatalf("selected kind = %q, want %q", got, repo.EntryKindWorktree)
	}
	if got := m.rows[m.cursor].entry.Path; got != "/repo/fix-render" {
		t.Fatalf("selected path = %q, want %q", got, "/repo/fix-render")
	}
}
