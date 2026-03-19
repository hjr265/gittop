package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
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

// LoadMailmap reads mailmap data from the repository. It loads the default
// .mailmap file from HEAD, then applies any additional mappings from the
// mailmap.file (filesystem path) and mailmap.blob (blob object) Git config
// options.
func LoadMailmap(repo *git.Repository) *Mailmap {
	m := &Mailmap{
		byEmail:     make(map[string]mailmapMailbox),
		byNameEmail: make(map[mailmapMailbox]mailmapMailbox),
	}

	// Load default .mailmap from HEAD tree.
	if ref, err := repo.Head(); err == nil {
		if commit, err := repo.CommitObject(ref.Hash()); err == nil {
			if tree, err := commit.Tree(); err == nil {
				if f, err := tree.File(".mailmap"); err == nil {
					if reader, err := f.Reader(); err == nil {
						if content, err := io.ReadAll(reader); err == nil {
							m.parse(content)
						}
						reader.Close()
					}
				}
			}
		}
	}

	// Check Git config for mailmap.file and mailmap.blob.
	cfg, err := repo.ConfigScoped(config.GlobalScope)
	if err != nil {
		return m
	}
	raw := cfg.Raw
	sec := raw.Section("mailmap")

	// mailmap.file: read from filesystem path.
	if filePath := sec.Option("file"); filePath != "" {
		if content, err := os.ReadFile(filePath); err == nil {
			m.parse(content)
		}
	}

	// mailmap.blob: read from a blob object.
	if blobRef := sec.Option("blob"); blobRef != "" {
		if hash, err := repo.ResolveRevision(plumbing.Revision(blobRef)); err == nil {
			if blob, err := repo.BlobObject(*hash); err == nil {
				if reader, err := blob.Reader(); err == nil {
					if content, err := io.ReadAll(reader); err == nil {
						m.parse(content)
					}
					reader.Close()
				}
			}
		}
	}

	return m
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
