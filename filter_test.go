package main

import (
	"testing"
)

func TestFilterParse(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`author:"hjr265" "Fix"`, `(author:"hjr265" and "Fix")`},
		{`author:"jskinner" and ("compile fix" or "bug") and path:*.cc`, `((author:"jskinner" and ("compile fix" or "bug")) and path:"*.cc")`},
		{`"Fix"`, `"Fix"`},
		{`path:*.go`, `path:"*.go"`},
		{`not author:foo`, `not author:"foo"`},
		{`author:hjr265`, `author:"hjr265"`},
		{`"feat" or "fix"`, `("feat" or "fix")`},
		{`not "wip"`, `not "wip"`},
		{`author:alice and author:bob`, `(author:"alice" and author:"bob")`},
		{`("a" or "b") and "c"`, `(("a" or "b") and "c")`},
	}
	for _, tt := range tests {
		expr, err := ParseFilter(tt.input)
		if err != nil {
			t.Errorf("ParseFilter(%q) error: %v", tt.input, err)
			continue
		}
		got := expr.String()
		if got != tt.want {
			t.Errorf("ParseFilter(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestFilterMatch(t *testing.T) {
	commit := CommitInfo{
		Author:  "hjr265",
		Email:   "m@hjr265.me",
		Message: "Fix login bug",
		Files:   []string{"cmd/main.go", "internal/auth.go"},
	}

	tests := []struct {
		input string
		want  bool
	}{
		{`author:"hjr265"`, true},
		{`author:"nobody"`, false},
		{`"Fix"`, true},
		{`"missing"`, false},
		{`author:"hjr265" "Fix"`, true},
		{`author:"hjr265" "missing"`, false},
		{`"Fix" or "missing"`, true},
		{`"missing" or "absent"`, false},
		{`not "missing"`, true},
		{`not "Fix"`, false},
		{`path:*.go`, true},
		{`path:*.cc`, false},
	}
	for _, tt := range tests {
		expr, err := ParseFilter(tt.input)
		if err != nil {
			t.Errorf("ParseFilter(%q) error: %v", tt.input, err)
			continue
		}
		got := expr.Match(&commit)
		if got != tt.want {
			t.Errorf("filter %q match = %v, want %v", tt.input, got, tt.want)
		}
	}
}
