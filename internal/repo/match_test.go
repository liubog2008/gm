package repo

import "testing"

func TestMatchPath(t *testing.T) {
	repos := []ManagedRepo{
		{
			Key:      "github.com/liubog2008/gm",
			Root:     "/base/github.com/liubog2008/gm",
			RepoPath: "liubog2008/gm",
			Worktrees: []Worktree{
				{Name: "main", Path: "/base/github.com/liubog2008/gm/main"},
				{Name: "feat-x", Path: "/base/github.com/liubog2008/gm/feat-x"},
			},
		},
	}

	tests := []struct {
		query string
		want  string
	}{
		{query: "gm", want: "/base/github.com/liubog2008/gm/main"},
		{query: "gm:main", want: "/base/github.com/liubog2008/gm/main"},
		{query: "liubog2008/gm:feat-x", want: "/base/github.com/liubog2008/gm/feat-x"},
		{query: "github.com/liubog2008/gm:feat-x", want: "/base/github.com/liubog2008/gm/feat-x"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got, err := MatchPath(repos, tt.query, nil, false, false)
			if err != nil {
				t.Fatalf("MatchPath() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("MatchPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchPathSplitRepoAndWorktreeFuzzy(t *testing.T) {
	repos := []ManagedRepo{
		{
			Key:      "github.com/acme/platform-gm",
			Root:     "/base/github.com/acme/platform-gm",
			RepoPath: "acme/platform-gm",
			Worktrees: []Worktree{
				{Name: "main", Path: "/base/github.com/acme/platform-gm/main"},
				{Name: "fix-render", Path: "/base/github.com/acme/platform-gm/fix-render"},
			},
		},
		{
			Key:      "github.com/acme/tooling",
			Root:     "/base/github.com/acme/tooling",
			RepoPath: "acme/tooling",
			Worktrees: []Worktree{
				{Name: "release", Path: "/base/github.com/acme/tooling/release"},
			},
		},
	}

	tests := []struct {
		query string
		want  string
	}{
		{query: "platform:fix", want: "/base/github.com/acme/platform-gm/fix-render"},
		{query: ":fix", want: "/base/github.com/acme/platform-gm/fix-render"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got, err := MatchPath(repos, tt.query, nil, false, false)
			if err != nil {
				t.Fatalf("MatchPath() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("MatchPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilterEntriesSplitQueryOnlyMatchesWorktrees(t *testing.T) {
	entries := []Entry{
		{Kind: EntryKindRepo, RepoKey: "github.com/acme/platform-gm", RepoPath: "acme/platform-gm", Label: "github.com/acme/platform-gm/.bare", Path: "/repo/.bare"},
		{Kind: EntryKindWorktree, RepoKey: "github.com/acme/platform-gm", RepoPath: "acme/platform-gm", Name: "fix-render", Branch: "feature/render-refresh", Label: "github.com/acme/platform-gm:fix-render", Path: "/repo/fix-render"},
	}

	got := FilterEntries(entries, EntryFilters{Query: "platform:fix"})
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Kind != EntryKindWorktree {
		t.Fatalf("got kind = %q, want %q", got[0].Kind, EntryKindWorktree)
	}
}

func TestFilterEntriesPlainQueryMatchesRepoAndWorktree(t *testing.T) {
	entries := []Entry{
		{Kind: EntryKindRepo, RepoKey: "github.com/acme/platform-gm", RepoPath: "acme/platform-gm", Label: "github.com/acme/platform-gm/.bare", Path: "/repo/.bare"},
		{Kind: EntryKindWorktree, RepoKey: "github.com/acme/platform-gm", RepoPath: "acme/platform-gm", Name: "main", Label: "github.com/acme/platform-gm:main", Path: "/repo/main"},
	}

	got := FilterEntries(entries, EntryFilters{Query: "platform"})
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
}

func TestFilterEntriesEmptyWorktreePartMatchesAllWorktrees(t *testing.T) {
	entries := []Entry{
		{Kind: EntryKindRepo, RepoKey: "github.com/acme/platform-gm", RepoPath: "acme/platform-gm", Label: "github.com/acme/platform-gm/.bare", Path: "/repo/.bare"},
		{Kind: EntryKindWorktree, RepoKey: "github.com/acme/platform-gm", RepoPath: "acme/platform-gm", Name: "main", Label: "github.com/acme/platform-gm:main", Path: "/repo/main"},
		{Kind: EntryKindWorktree, RepoKey: "github.com/acme/platform-gm", RepoPath: "acme/platform-gm", Name: "fix-render", Label: "github.com/acme/platform-gm:fix-render", Path: "/repo/fix-render"},
		{Kind: EntryKindWorktree, RepoKey: "github.com/acme/tooling", RepoPath: "acme/tooling", Name: "release", Label: "github.com/acme/tooling:release", Path: "/tooling/release"},
	}

	got := FilterEntries(entries, EntryFilters{Query: ":"})
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	for _, entry := range got {
		if entry.Kind != EntryKindWorktree {
			t.Fatalf("got kind = %q, want only worktrees", entry.Kind)
		}
	}

	got = FilterEntries(entries, EntryFilters{Query: "platform:"})
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	for _, entry := range got {
		if entry.RepoKey != "github.com/acme/platform-gm" || entry.Kind != EntryKindWorktree {
			t.Fatalf("got unexpected entry: %#v", entry)
		}
	}
}

func TestMatchPathAmbiguous(t *testing.T) {
	repos := []ManagedRepo{
		{Key: "github.com/acme/gm", Root: "/base/github.com/acme/gm", RepoPath: "acme/gm", Worktrees: []Worktree{{Name: "main", Path: "/base/github.com/acme/gm/main"}}},
		{Key: "github.com/liubog2008/gm", Root: "/base/github.com/liubog2008/gm", RepoPath: "liubog2008/gm", Worktrees: []Worktree{{Name: "main", Path: "/base/github.com/liubog2008/gm/main"}}},
	}

	if _, err := MatchPath(repos, "gm", nil, false, false); err == nil {
		t.Fatalf("MatchPath() expected ambiguous error")
	}
}

func TestFilterEntriesIncludesBareAndWorktree(t *testing.T) {
	repos := []ManagedRepo{
		{
			Key:      "github.com/liubog2008/gm",
			Root:     "/base/github.com/liubog2008/gm",
			RepoPath: "liubog2008/gm",
			Worktrees: []Worktree{
				{Name: "main", Path: "/base/github.com/liubog2008/gm/main"},
			},
		},
	}

	entries := FilterEntries(BuildEntries(repos), EntryFilters{})
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	foundRepo := false
	foundWorktree := false
	for _, entry := range entries {
		switch {
		case entry.Kind == EntryKindRepo && entry.Path == "/base/github.com/liubog2008/gm/.bare":
			foundRepo = true
		case entry.Kind == EntryKindWorktree && entry.Path == "/base/github.com/liubog2008/gm/main":
			foundWorktree = true
		}
	}
	if !foundRepo || !foundWorktree {
		t.Fatalf("entries missing expected repo/worktree: %#v", entries)
	}
}

func TestFilterEntriesRecentFirst(t *testing.T) {
	entries := []Entry{
		{Kind: EntryKindWorktree, Label: "github.com/acme/demo:main", Path: "/a"},
		{Kind: EntryKindWorktree, Label: "github.com/acme/demo:feat", Path: "/b"},
	}

	got := FilterEntries(entries, EntryFilters{
		RecentWeights: map[string]int64{
			"/b": 20,
			"/a": 10,
		},
	})
	if got[0].Path != "/b" {
		t.Fatalf("got first path %q, want /b", got[0].Path)
	}
}
