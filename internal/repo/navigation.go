package repo

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type EntryKind string

const (
	EntryKindRepo     EntryKind = "repo"
	EntryKindWorktree EntryKind = "worktree"
)

type Entry struct {
	Kind      EntryKind
	RepoKey   string
	RepoPath  string
	Name      string
	Label     string
	Path      string
	Branch    string
	RemoteURL string
	Legacy    bool
}

type EntryFilters struct {
	Query         string
	OnlyRepo      bool
	OnlyWorktree  bool
	RecentWeights map[string]int64
}

func BuildEntries(repos []ManagedRepo) []Entry {
	out := make([]Entry, 0, len(repos)*2)
	for _, item := range repos {
		out = append(out, repoEntry(item))
		for _, wt := range item.Worktrees {
			out = append(out, Entry{
				Kind:      EntryKindWorktree,
				RepoKey:   item.Key,
				RepoPath:  item.RepoPath,
				Name:      wt.Name,
				Label:     item.Key + ":" + wt.Name,
				Path:      wt.Path,
				Branch:    wt.Branch,
				RemoteURL: item.RemoteURL,
				Legacy:    item.Legacy,
			})
		}
	}
	return out
}

func FilterEntries(entries []Entry, filters EntryFilters) []Entry {
	query := parseQuery(filters.Query)
	out := make([]Entry, 0, len(entries))
	for _, item := range entries {
		if filters.OnlyRepo && item.Kind != EntryKindRepo {
			continue
		}
		if filters.OnlyWorktree && item.Kind != EntryKindWorktree {
			continue
		}
		if query.raw != "" {
			score := matchParsedQuery(query, item)
			if score.exactness == 0 {
				continue
			}
		}
		out = append(out, item)
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		sa := rankEntry(query, a, filters.RecentWeights)
		sb := rankEntry(query, b, filters.RecentWeights)
		if sa.exactness != sb.exactness {
			return sa.exactness > sb.exactness
		}
		if sa.recent != sb.recent {
			return sa.recent > sb.recent
		}
		if sa.kind != sb.kind {
			return sa.kind > sb.kind
		}
		if sa.length != sb.length {
			return sa.length < sb.length
		}
		return a.Label < b.Label
	})
	return out
}

func MatchPath(repos []ManagedRepo, query string, recent map[string]int64, onlyRepo, onlyWorktree bool) (string, error) {
	parsed := parseQuery(query)
	candidates := FilterEntries(BuildEntries(repos), EntryFilters{
		Query:         parsed.raw,
		OnlyRepo:      onlyRepo,
		OnlyWorktree:  onlyWorktree,
		RecentWeights: recent,
	})
	if len(candidates) == 0 {
		return "", fmt.Errorf("no match for %q", strings.TrimSpace(query))
	}

	best := candidates[0]
	if len(candidates) > 1 {
		a := rankEntry(parsed, candidates[0], recent)
		b := rankEntry(parsed, candidates[1], recent)
		if a.exactness == b.exactness && a.recent == b.recent && a.kind == b.kind {
			return "", ambiguousError(strings.TrimSpace(query), candidates)
		}
	}
	return best.Path, nil
}

type score struct {
	exactness int
	recent    int64
	kind      int
	length    int
}

func rankEntry(query parsedQuery, item Entry, recent map[string]int64) score {
	score := score{
		kind:   kindRank(item.Kind),
		length: len(item.Label),
	}
	if recent != nil {
		score.recent = recent[item.Path]
	}
	if query.raw != "" {
		score.exactness = matchParsedQuery(query, item).exactness
	}
	return score
}

func kindRank(kind EntryKind) int {
	if kind == EntryKindWorktree {
		return 2
	}
	return 1
}

func matchEntry(query string, item Entry) score {
	return matchScore(query, entryForms(item))
}

func repoForms(item Entry) []string {
	return compactForms([]string{
		item.RepoKey,
		item.RepoPath,
		lastSegment(item.RepoPath),
	})
}

func worktreeForms(item Entry) []string {
	if item.Kind != EntryKindWorktree {
		return nil
	}
	forms := []string{
		item.Name,
		item.Branch,
	}
	if item.Name == DefaultWorktreeName {
		forms = append(forms, "main")
	}
	return compactForms(forms)
}

func entryForms(item Entry) []string {
	if item.Kind == EntryKindRepo {
		return compactForms(append([]string{item.Label}, repoForms(item)...))
	}
	forms := []string{
		item.Label,
		item.RepoKey + ":" + item.Name,
		item.RepoPath + ":" + item.Name,
		lastSegment(item.RepoPath) + ":" + item.Name,
		lastSegment(item.RepoPath) + "/" + item.Name,
		item.Name,
	}
	if item.Name == DefaultWorktreeName {
		forms = append(forms, item.RepoKey, item.RepoPath, lastSegment(item.RepoPath))
	}
	return compactForms(append(forms, worktreeForms(item)...))
}

func compactForms(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func repoEntry(item ManagedRepo) Entry {
	path := item.Root
	name := "repo"
	label := item.Key
	if !item.Legacy {
		path = filepath.Join(item.Root, ".bare")
		name = ".bare"
		label = item.Key + "/.bare"
	}
	return Entry{
		Kind:      EntryKindRepo,
		RepoKey:   item.Key,
		RepoPath:  item.RepoPath,
		Name:      name,
		Label:     label,
		Path:      path,
		RemoteURL: item.RemoteURL,
		Legacy:    item.Legacy,
	}
}
