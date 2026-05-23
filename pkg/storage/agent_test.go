package storage

import (
	"os"
	"testing"
)

func TestAgentPATEncryption(t *testing.T) {
	// Set encryption key for test
	os.Setenv("FLOW_SECRET_KEY", "test-secret-key-12345")

	tests := []struct {
		name    string
		pat     string
		wantErr bool
	}{
		{
			name: "encrypt and decrypt PAT",
			pat:  "ghp_test123456789abcdefghijklmnop",
		},
		{
			name: "empty PAT",
			pat:  "",
		},
		{
			name: "long PAT",
			pat:  "ghp_" + string(make([]byte, 1000)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{
				ID:   "test-agent",
				Name: "test",
			}

			// SetPAT should encrypt
			if err := a.SetPAT(tt.pat); (err != nil) != tt.wantErr {
				t.Errorf("SetPAT() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// For non-empty PATs, plaintext should be cleared and sealed should be populated
			if tt.pat != "" {
				if a.PAT != "" {
					t.Errorf("PAT field should be empty after SetPAT")
				}
				if a.PATSealed == "" {
					t.Errorf("PATSealed should be populated")
				}
			} else {
				// For empty PATs, both should be empty
				if a.PAT != "" || a.PATSealed != "" {
					t.Errorf("Both PAT and PATSealed should be empty for empty input")
				}
			}

			// GetPAT should decrypt and match original
			got, err := a.GetPAT()
			if err != nil {
				t.Errorf("GetPAT() error = %v", err)
				return
			}
			if got != tt.pat {
				t.Errorf("GetPAT() = %q, want %q", got, tt.pat)
			}
		})
	}
}

func TestAgentPATBackwardCompatibility(t *testing.T) {
	// Set encryption key for test
	os.Setenv("FLOW_SECRET_KEY", "test-secret-key-12345")

	t.Run("plaintext PAT fallback", func(t *testing.T) {
		plainPAT := "ghp_oldplaintexttoken123"
		a := &Agent{
			ID:  "old-agent",
			PAT: plainPAT,
			// PATSealed intentionally empty to simulate pre-encryption agent
		}

		// GetPAT should return plaintext PAT
		got, err := a.GetPAT()
		if err != nil {
			t.Errorf("GetPAT() error = %v", err)
			return
		}
		if got != plainPAT {
			t.Errorf("GetPAT() = %q, want %q", got, plainPAT)
		}
	})

	t.Run("sealed PAT takes precedence", func(t *testing.T) {
		a := &Agent{
			ID: "test-agent",
		}

		// First set the sealed PAT
		if err := a.SetPAT("ghp_newencrypted"); err != nil {
			t.Fatalf("SetPAT() error = %v", err)
		}

		// Try to confuse GetPAT by adding plaintext
		a.PAT = "ghp_should_not_use_this"

		// GetPAT should use PATSealed, not plaintext
		got, err := a.GetPAT()
		if err != nil {
			t.Errorf("GetPAT() error = %v", err)
			return
		}
		if got != "ghp_newencrypted" {
			t.Errorf("GetPAT() = %q, want %q", got, "ghp_newencrypted")
		}
	})
}

// Note: TestAgentPATNoKeyError is skipped because secrets.loadKey() caches the key
// via sync.Once, so we can't test the no-key error path without restarting the process.
// The error handling is covered by integration tests and the API layer returns
// errors to clients when credential writes are attempted without FLOW_SECRET_KEY.
