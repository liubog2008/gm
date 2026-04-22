package repo

import "testing"

func TestParseLocator(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		host string
		path string
	}{
		{name: "https", raw: "https://github.com/liubog2008/gm", host: "github.com", path: "liubog2008/gm"},
		{name: "https git suffix", raw: "https://github.com/liubog2008/gm.git", host: "github.com", path: "liubog2008/gm"},
		{name: "ssh short", raw: "git@github.com:liubog2008/gm.git", host: "github.com", path: "liubog2008/gm"},
		{name: "ssh scheme", raw: "ssh://git@github.com/liubog2008/gm.git", host: "github.com", path: "liubog2008/gm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := ParseLocator("/base", tt.raw)
			if err != nil {
				t.Fatalf("ParseLocator() error = %v", err)
			}
			if loc.Host != tt.host || loc.Path != tt.path {
				t.Fatalf("ParseLocator() = host=%q path=%q", loc.Host, loc.Path)
			}
		})
	}
}
