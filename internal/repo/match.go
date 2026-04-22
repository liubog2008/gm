package repo

import (
	"errors"
	"fmt"
	"strings"
)

type parsedQuery struct {
	raw           string
	repoPart      string
	worktreePart  string
	hasSeparator  bool
}

func parseQuery(query string) parsedQuery {
	query = strings.TrimSpace(strings.Trim(query, "/"))
	parts := strings.SplitN(query, ":", 2)
	if len(parts) == 1 {
		return parsedQuery{raw: query}
	}
	return parsedQuery{
		raw:          query,
		repoPart:     strings.TrimSpace(parts[0]),
		worktreePart: strings.TrimSpace(parts[1]),
		hasSeparator: true,
	}
}

func matchScore(query string, forms []string) score {
	best := score{}
	for _, form := range forms {
		switch {
		case form == query:
			if best.exactness < 4 {
				best = score{exactness: 4}
			}
		case strings.HasSuffix(form, ":"+query):
			if best.exactness < 3 {
				best = score{exactness: 3}
			}
		case strings.HasSuffix(form, query):
			if best.exactness < 2 {
				best = score{exactness: 2}
			}
		case strings.Contains(form, query):
			if best.exactness < 1 {
				best = score{exactness: 1}
			}
		}
	}
	return best
}

func matchRepoScore(query string, item Entry) score {
	return matchScore(query, repoForms(item))
}

func matchWorktreeScore(query string, item Entry) score {
	if item.Kind != EntryKindWorktree {
		return score{}
	}
	return matchScore(query, worktreeForms(item))
}

func matchParsedQuery(query parsedQuery, item Entry) score {
	if !query.hasSeparator {
		return matchEntry(query.raw, item)
	}

	if item.Kind != EntryKindWorktree {
		return score{}
	}

	total := score{}
	if query.repoPart != "" {
		repoScore := matchRepoScore(query.repoPart, item)
		if repoScore.exactness == 0 {
			return score{}
		}
		total.exactness += repoScore.exactness * 10
	}
	if query.worktreePart != "" {
		worktreeScore := matchWorktreeScore(query.worktreePart, item)
		if worktreeScore.exactness == 0 {
			return score{}
		}
		total.exactness += worktreeScore.exactness
	} else {
		total.exactness++
	}
	if query.repoPart == "" && query.worktreePart == "" {
		return total
	}
	return total
}

func ambiguousError(query string, candidates []Entry) error {
	lines := []string{fmt.Sprintf("ambiguous query %q, candidates:", query)}
	seen := map[string]struct{}{}
	for _, c := range candidates {
		if _, ok := seen[c.Label]; ok {
			continue
		}
		seen[c.Label] = struct{}{}
		lines = append(lines, "- "+c.Label)
		if len(lines) >= 6 {
			break
		}
	}
	return errors.New(strings.Join(lines, "\n"))
}

func lastSegment(s string) string {
	parts := strings.Split(strings.Trim(s, "/"), "/")
	if len(parts) == 0 {
		return s
	}
	return parts[len(parts)-1]
}
