package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type recentStore struct {
	Items []recentItem `json:"items"`
}

type recentItem struct {
	Path     string `json:"path"`
	LastUsed int64  `json:"last_used"`
}

func loadRecent(path string) (map[string]int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]int64{}, nil
		}
		return nil, err
	}

	var store recentStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(store.Items))
	for _, item := range store.Items {
		if item.Path == "" || item.LastUsed == 0 {
			continue
		}
		out[item.Path] = item.LastUsed
	}
	return out, nil
}

func saveRecent(path string, items map[string]int64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	store := recentStore{Items: make([]recentItem, 0, len(items))}
	for k, v := range items {
		if k == "" || v == 0 {
			continue
		}
		store.Items = append(store.Items, recentItem{Path: k, LastUsed: v})
	}
	sort.Slice(store.Items, func(i, j int) bool {
		return store.Items[i].LastUsed > store.Items[j].LastUsed
	})
	if len(store.Items) > 200 {
		store.Items = store.Items[:200]
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func markRecent(path string, items map[string]int64) map[string]int64 {
	if items == nil {
		items = map[string]int64{}
	}
	items[path] = time.Now().Unix()
	return items
}
