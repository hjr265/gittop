package main

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	"github.com/go-git/go-git/v5"
)

// Mailmap maps commit author name/email pairs to canonical ones.
type Mailmap struct {
	// byEmail maps a commit email to a canonical name and email.
	// Key: lowercase commit email.
	byEmail map[string]mailmapMailbox

	// byNameEmail maps a (commit name, commit email) pair to canonical values.
	// Key: "name\x00email" (both lowercased).
	byNameEmail map[mailmapMailbox]mailmapMailbox
}

type mailmapMailbox struct {
	name  string
	email string
}

// LoadMailmap reads the .mailmap file from the repository's HEAD tree.
// Returns an empty (no-op) Mailmap if the file doesn't exist.
func LoadMailmap(repo *git.Repository) *Mailmap {
	m := &Mailmap{
		byEmail:     make(map[string]mailmapMailbox),
		byNameEmail: make(map[mailmapMailbox]mailmapMailbox),
	}

	ref, err := repo.Head()
	if err != nil {
		return m
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return m
	}
	tree, err := commit.Tree()
	if err != nil {
		return m
	}
	f, err := tree.File(".mailmap")
	if err != nil {
		return m
	}
	reader, err := f.Reader()
	if err != nil {
		return m
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return m
	}

	m.parse(content)
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
