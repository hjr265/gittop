package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Grammar AST for participle.

type Query struct {
	Or *OrExpr `parser:"@@"`
}

type OrExpr struct {
	Left  *AndExpr   `parser:"@@"`
	Right []*AndExpr `parser:"( 'or' @@ )*"`
}

type AndExpr struct {
	Left  *UnaryExpr   `parser:"@@"`
	Right []*UnaryExpr `parser:"( ( 'and' )? @@ )*"`
}

type UnaryExpr struct {
	Not     *UnaryExpr  `parser:"  'not' @@"`
	Field   *FieldExpr  `parser:"| @@"`
	Group   *OrExpr     `parser:"| '(' @@ ')'"`
	Message *string     `parser:"| @(String | Ident)"`
}

type FieldExpr struct {
	Author *string `parser:"'author' ':' @(String | Ident | Pattern)"`
	Path   *string `parser:"| 'path' ':' @(String | Ident | Pattern)"`
	Branch *string `parser:"| 'branch' ':' @(String | Ident | Pattern)"`
}

var filterLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Whitespace", Pattern: `\s+`},
	{Name: "String", Pattern: `"[^"]*"`},
	{Name: "Keyword", Pattern: `(?i)(?:and|or|not)\b`},
	{Name: "Pattern", Pattern: `[a-zA-Z0-9_\-]*[.*/?][a-zA-Z0-9_\-.*/?]*`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_\-]*`},
	{Name: "Punct", Pattern: `[():]`},
})

var filterParser = participle.MustBuild[Query](
	participle.Lexer(filterLexer),
	participle.Elide("Whitespace"),
	participle.CaseInsensitive("Keyword", "Ident"),
	participle.Unquote("String"),
)

// ParseFilter parses a filter query string into a FilterExpr.
func ParseFilter(input string) (FilterExpr, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}
	query, err := filterParser.ParseString("", input)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return compileOr(query.Or), nil
}

// Compile AST nodes into FilterExpr evaluation nodes.

func compileOr(node *OrExpr) FilterExpr {
	if node == nil {
		return nil
	}
	result := compileAnd(node.Left)
	for _, right := range node.Right {
		result = &orFilterExpr{left: result, right: compileAnd(right)}
	}
	return result
}

func compileAnd(node *AndExpr) FilterExpr {
	if node == nil {
		return nil
	}
	result := compileUnary(node.Left)
	for _, right := range node.Right {
		result = &andFilterExpr{left: result, right: compileUnary(right)}
	}
	return result
}

func compileUnary(node *UnaryExpr) FilterExpr {
	if node == nil {
		return nil
	}
	if node.Not != nil {
		return &notFilterExpr{inner: compileUnary(node.Not)}
	}
	if node.Field != nil {
		return compileField(node.Field)
	}
	if node.Group != nil {
		return compileOr(node.Group)
	}
	if node.Message != nil {
		return &messageFilterExpr{text: *node.Message}
	}
	return nil
}

func compileField(node *FieldExpr) FilterExpr {
	if node.Author != nil {
		return &authorFilterExpr{pattern: *node.Author}
	}
	if node.Path != nil {
		return &pathFilterExpr{pattern: *node.Path}
	}
	if node.Branch != nil {
		return &branchFilterExpr{pattern: *node.Branch}
	}
	return nil
}

// FilterExpr evaluation types.

type FilterExpr interface {
	Match(c *CommitInfo) bool
	String() string
}

type andFilterExpr struct {
	left, right FilterExpr
}

func (e *andFilterExpr) Match(c *CommitInfo) bool { return e.left.Match(c) && e.right.Match(c) }
func (e *andFilterExpr) String() string           { return fmt.Sprintf("(%s and %s)", e.left, e.right) }

type orFilterExpr struct {
	left, right FilterExpr
}

func (e *orFilterExpr) Match(c *CommitInfo) bool { return e.left.Match(c) || e.right.Match(c) }
func (e *orFilterExpr) String() string           { return fmt.Sprintf("(%s or %s)", e.left, e.right) }

type notFilterExpr struct {
	inner FilterExpr
}

func (e *notFilterExpr) Match(c *CommitInfo) bool { return !e.inner.Match(c) }
func (e *notFilterExpr) String() string           { return fmt.Sprintf("not %s", e.inner) }

type authorFilterExpr struct {
	pattern string
}

func (e *authorFilterExpr) Match(c *CommitInfo) bool {
	p := strings.ToLower(e.pattern)
	return strings.Contains(strings.ToLower(c.Author), p) ||
		strings.Contains(strings.ToLower(c.Email), p)
}
func (e *authorFilterExpr) String() string { return fmt.Sprintf("author:%q", e.pattern) }

type pathFilterExpr struct {
	pattern string
}

func (e *pathFilterExpr) Match(c *CommitInfo) bool {
	p := strings.ToLower(e.pattern)
	for _, f := range c.Files {
		matched, _ := filepath.Match(p, strings.ToLower(f))
		if matched {
			return true
		}
		matched, _ = filepath.Match(p, strings.ToLower(filepath.Base(f)))
		if matched {
			return true
		}
		if strings.Contains(strings.ToLower(f), p) {
			return true
		}
	}
	return false
}
func (e *pathFilterExpr) String() string { return fmt.Sprintf("path:%q", e.pattern) }

type messageFilterExpr struct {
	text string
}

func (e *messageFilterExpr) Match(c *CommitInfo) bool {
	return strings.Contains(strings.ToLower(c.Message), strings.ToLower(e.text))
}
func (e *messageFilterExpr) String() string { return fmt.Sprintf("%q", e.text) }

type branchFilterExpr struct {
	pattern string
	hashes  map[string]bool // populated by PopulateBranchHashes before filtering
}

func (e *branchFilterExpr) Match(c *CommitInfo) bool {
	return e.hashes[c.Hash]
}
func (e *branchFilterExpr) String() string { return fmt.Sprintf("branch:%q", e.pattern) }

// FilterNeedsFiles returns true if the filter expression requires file lists.
func FilterNeedsFiles(expr FilterExpr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *pathFilterExpr:
		return true
	case *andFilterExpr:
		return FilterNeedsFiles(e.left) || FilterNeedsFiles(e.right)
	case *orFilterExpr:
		return FilterNeedsFiles(e.left) || FilterNeedsFiles(e.right)
	case *notFilterExpr:
		return FilterNeedsFiles(e.inner)
	default:
		return false
	}
}

// FilterNeedsBranches returns true if the filter expression uses branch: filters.
func FilterNeedsBranches(expr FilterExpr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *branchFilterExpr:
		return true
	case *andFilterExpr:
		return FilterNeedsBranches(e.left) || FilterNeedsBranches(e.right)
	case *orFilterExpr:
		return FilterNeedsBranches(e.left) || FilterNeedsBranches(e.right)
	case *notFilterExpr:
		return FilterNeedsBranches(e.inner)
	default:
		return false
	}
}

// PopulateBranchHashes walks all branches in the repo and populates the hash
// sets in any branchFilterExpr nodes within the expression tree.
func PopulateBranchHashes(expr FilterExpr, repo *git.Repository) {
	if expr == nil || repo == nil {
		return
	}

	var nodes []*branchFilterExpr
	collectBranchNodes(expr, &nodes)
	if len(nodes) == 0 {
		return
	}

	for _, n := range nodes {
		n.hashes = make(map[string]bool)
	}

	branches, err := repo.Branches()
	if err != nil {
		return
	}
	branches.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()

		// Check which filter nodes match this branch name.
		var matching []*branchFilterExpr
		for _, n := range nodes {
			p := strings.ToLower(n.pattern)
			nameLower := strings.ToLower(name)
			matched, _ := filepath.Match(p, nameLower)
			if matched || strings.Contains(nameLower, p) {
				matching = append(matching, n)
			}
		}
		if len(matching) == 0 {
			return nil
		}

		// Walk commits reachable from this branch.
		iter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
		if err != nil {
			return nil
		}
		iter.ForEach(func(c *object.Commit) error {
			h := c.Hash.String()
			for _, n := range matching {
				n.hashes[h] = true
			}
			return nil
		})

		return nil
	})
}

func collectBranchNodes(expr FilterExpr, nodes *[]*branchFilterExpr) {
	switch e := expr.(type) {
	case *branchFilterExpr:
		*nodes = append(*nodes, e)
	case *andFilterExpr:
		collectBranchNodes(e.left, nodes)
		collectBranchNodes(e.right, nodes)
	case *orFilterExpr:
		collectBranchNodes(e.left, nodes)
		collectBranchNodes(e.right, nodes)
	case *notFilterExpr:
		collectBranchNodes(e.inner, nodes)
	}
}
