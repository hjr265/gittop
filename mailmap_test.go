package main

import "testing"

func TestMailmapResolve(t *testing.T) {
	content := []byte(`# comment
Proper Name <proper@example.com>
<canonical@example.com> <old@example.com>
Full Name <full@example.com> <alt@example.com>
Real Name <real@example.com> Typo Name <typo@example.com>
`)

	m := &Mailmap{
		byEmail:     make(map[string]mailmapMailbox),
		byNameEmail: make(map[mailmapMailbox]mailmapMailbox),
	}
	m.parse(content)

	tests := []struct {
		inName, inEmail     string
		wantName, wantEmail string
	}{
		// Format 1: "Proper Name <proper@example.com>" — replaces name for matching email.
		{"Old Name", "proper@example.com", "Proper Name", "proper@example.com"},
		// Format 2: "<canonical@example.com> <old@example.com>" — replaces email.
		{"Someone", "old@example.com", "Someone", "canonical@example.com"},
		// Format 3: "Full Name <full@example.com> <alt@example.com>" — replaces both.
		{"Whatever", "alt@example.com", "Full Name", "full@example.com"},
		// Format 4: "Real Name <real@example.com> Typo Name <typo@example.com>" — specific match.
		{"Typo Name", "typo@example.com", "Real Name", "real@example.com"},
		// Format 4 should not match different name with same email.
		{"Other Name", "typo@example.com", "Other Name", "typo@example.com"},
		// No match — pass through.
		{"Unknown", "unknown@example.com", "Unknown", "unknown@example.com"},
		// Case-insensitive email match.
		{"Old Name", "PROPER@EXAMPLE.COM", "Proper Name", "proper@example.com"},
	}

	for _, tt := range tests {
		gotName, gotEmail := m.Resolve(tt.inName, tt.inEmail)
		if gotName != tt.wantName || gotEmail != tt.wantEmail {
			t.Errorf("Resolve(%q, %q) = (%q, %q), want (%q, %q)",
				tt.inName, tt.inEmail, gotName, gotEmail, tt.wantName, tt.wantEmail)
		}
	}
}

func TestMailmapNil(t *testing.T) {
	var m *Mailmap
	name, email := m.Resolve("Test", "test@example.com")
	if name != "Test" || email != "test@example.com" {
		t.Errorf("nil Mailmap should pass through, got (%q, %q)", name, email)
	}
}
