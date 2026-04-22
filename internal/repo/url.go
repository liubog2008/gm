package repo

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

type Locator struct {
	Host string
	Path string
	Root string
}

func ParseLocator(baseDir, raw string) (Locator, error) {
	host, repoPath, err := parseURL(raw)
	if err != nil {
		return Locator{}, err
	}
	root := filepath.Join(append([]string{baseDir, host}, strings.Split(repoPath, "/")...)...)
	return Locator{
		Host: host,
		Path: repoPath,
		Root: root,
	}, nil
}

func parseURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("empty repo url")
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", fmt.Errorf("parse repo url: %w", err)
		}
		repoPath := strings.TrimPrefix(u.Path, "/")
		repoPath = strings.TrimSuffix(repoPath, ".git")
		repoPath = path.Clean(repoPath)
		if u.Host == "" || repoPath == "." || repoPath == "" {
			return "", "", fmt.Errorf("invalid repo url %q", raw)
		}
		if err := validateRepoPath(repoPath); err != nil {
			return "", "", err
		}
		return strings.ToLower(u.Host), repoPath, nil
	}

	if at := strings.Index(raw, "@"); at >= 0 {
		hostPath := raw[at+1:]
		parts := strings.SplitN(hostPath, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid ssh repo url %q", raw)
		}
		host := strings.ToLower(parts[0])
		repoPath := strings.TrimSuffix(parts[1], ".git")
		repoPath = path.Clean(repoPath)
		if err := validateRepoPath(repoPath); err != nil {
			return "", "", err
		}
		return host, repoPath, nil
	}

	return "", "", fmt.Errorf("unsupported repo url %q", raw)
}

func validateRepoPath(repoPath string) error {
	parts := strings.Split(repoPath, "/")
	if len(parts) < 2 {
		return fmt.Errorf("repo path must include owner/repo: %q", repoPath)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid repo path %q", repoPath)
		}
	}
	return nil
}
