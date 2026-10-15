package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// Mailmap maps commit author name/email pairs to canonical ones.
type Mailmap struct {
	// byEmail maps a commit email to a canonical name and email.
	// Key: lowercase commit email.
	byEmail map[string]mailmapMailbox

	// byNameEmail maps a (commit name, commit email) pair to canonical values.
	// Key: "{name email}" (both lowercased).
	byNameEmail map[mailmapMailbox]mailmapMailbox
}

type mailmapMailbox struct {
	name  string
	email string
}

// LoadMailmap reads mailmap data the way Git does. In a repository with a
// working tree it reads the .mailmap file there; in a bare one it falls back
// to the .mailmap committed at HEAD. It then applies the mailmap.blob and
// mailmap.file Git config options, in that order, so the file has the last
// word.
func LoadMailmap(repo *git.Repository) *Mailmap {
	m := &Mailmap{
		byEmail:     make(map[string]mailmapMailbox),
		byNameEmail: make(map[mailmapMailbox]mailmapMailbox),
	}

	blobRev, filePath := mailmapConfig(repo)
	root := ""

	if wt, err := repo.Worktree(); err == nil {
		root = wt.Filesystem.Root()
		// Like Git, do not follow a symlinked .mailmap.
		if fi, err := wt.Filesystem.Lstat(".mailmap"); err == nil && fi.Mode().IsRegular() {
			if f, err := wt.Filesystem.Open(".mailmap"); err == nil {
				if content, err := io.ReadAll(f); err == nil {
					m.parse(content)
				}
				f.Close()
			}
		}
	} else if blobRev == "" {
		blobRev = "HEAD:.mailmap"
	}

	if blobRev != "" {
		if content, err := readRevisionBlob(repo, blobRev); err == nil {
			m.parse(content)
		}
	}

	if filePath != "" {
		if content, err := os.ReadFile(expandConfigPath(filePath, root)); err == nil {
			m.parse(content)
		}
	}

	return m
}

// mailmapConfig returns the mailmap.blob and mailmap.file Git config options.
// The repository's own config takes precedence over the user's, and the
// user's over the system's.
func mailmapConfig(repo *git.Repository) (blob, file string) {
	var cfgs []*config.Config
	if cfg, err := repo.Config(); err == nil {
		cfgs = append(cfgs, cfg)
	}
	for _, scope := range []config.Scope{config.GlobalScope, config.SystemScope} {
		if cfg, err := config.LoadConfig(scope); err == nil {
			cfgs = append(cfgs, cfg)
		}
	}

	for _, cfg := range cfgs {
		if cfg.Raw == nil || !cfg.Raw.HasSection("mailmap") {
			continue
		}
		sec := cfg.Raw.Section("mailmap")
		if blob == "" {
			blob = sec.Option("blob")
		}
		if file == "" {
			file = sec.Option("file")
		}
	}
	return blob, file
}

// readRevisionBlob reads the blob named by a revision: either
// "<commit>:<path>", such as "HEAD:.mailmap", or anything that resolves to a
// blob directly.
func readRevisionBlob(repo *git.Repository, rev string) ([]byte, error) {
	if i := strings.Index(rev, ":"); i >= 0 {
		hash, err := repo.ResolveRevision(plumbing.Revision(rev[:i]))
		if err != nil {
			return nil, err
		}
		commit, err := repo.CommitObject(*hash)
		if err != nil {
			return nil, err
		}
		f, err := commit.File(rev[i+1:])
		if err != nil {
			return nil, err
		}
		content, err := f.Contents()
		return []byte(content), err
	}

	hash, err := repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return nil, err
	}
	blob, err := repo.BlobObject(*hash)
	if err != nil {
		return nil, err
	}
	reader, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// expandConfigPath resolves a path from Git config: a leading "~/" is the
// user's home directory, and a relative path is relative to root, if given.
func expandConfigPath(path, root string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
		return path
	}
	if !filepath.IsAbs(path) && root != "" {
		return filepath.Join(root, path)
	}
	return path
}

// parse processes .mailmap file content.
//
// Supported formats (see https://git-scm.com/docs/gitmailmap):
//
//	Proper Name <proper@email>
//	<proper@email> <commit@email>
//	Proper Name <proper@email> <commit@email>
//	Proper Name <proper@email> Commit Name <commit@email>
func (m *Mailmap) parse(content []byte) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}

		// Extract all <email> tokens and the text before each.
		var emails []string
		var names []string
		remaining := line

		for {
			open := strings.IndexByte(remaining, '<')
			if open < 0 {
				break
			}
			close := strings.IndexByte(remaining[open:], '>')
			if close < 0 {
				break
			}
			close += open

			names = append(names, strings.TrimSpace(remaining[:open]))
			emails = append(emails, strings.TrimSpace(remaining[open+1:close]))
			remaining = remaining[close+1:]
		}

		if len(emails) == 0 {
			continue
		}

		switch len(emails) {
		case 1:
			// "Proper Name <proper@email>" — map by proper email to proper name.
			properName := names[0]
			properEmail := emails[0]
			if properName != "" {
				m.byEmail[strings.ToLower(properEmail)] = mailmapMailbox{
					name:  properName,
					email: properEmail,
				}
			}

		case 2:
			properName := names[0]
			properEmail := emails[0]
			commitName := names[1]
			commitEmail := emails[1]

			entry := mailmapMailbox{
				name:  properName,
				email: properEmail,
			}

			if commitName != "" {
				// "Proper Name <proper@email> Commit Name <commit@email>"
				key := mailmapMailbox{strings.ToLower(commitName), strings.ToLower(commitEmail)}
				m.byNameEmail[key] = entry

			} else {
				// "<proper@email> <commit@email>" or "Proper Name <proper@email> <commit@email>"
				m.byEmail[strings.ToLower(commitEmail)] = entry
			}
		}
	}
}

// Resolve maps a commit name and email to their canonical forms.
func (m *Mailmap) Resolve(name, email string) (string, string) {
	if m == nil || (len(m.byEmail) == 0 && len(m.byNameEmail) == 0) {
		return name, email
	}

	// Try the more specific (name, email) match first.
	key := mailmapMailbox{strings.ToLower(name), strings.ToLower(email)}
	if entry, ok := m.byNameEmail[key]; ok {
		resolved := name
		if entry.name != "" {
			resolved = entry.name
		}
		resolvedEmail := email
		if entry.email != "" {
			resolvedEmail = entry.email
		}
		return resolved, resolvedEmail
	}

	// Fall back to email-only match.
	if entry, ok := m.byEmail[strings.ToLower(email)]; ok {
		resolved := name
		if entry.name != "" {
			resolved = entry.name
		}
		resolvedEmail := email
		if entry.email != "" {
			resolvedEmail = entry.email
		}
		return resolved, resolvedEmail
	}

	return name, email
}
