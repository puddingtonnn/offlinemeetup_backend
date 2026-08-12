package mail

import (
	"strings"
	"testing"
	"time"
)

const testCodeTTL = 15 * time.Minute

func TestRegistrationVerification_ContainsUsernameAndCode(t *testing.T) {
	subject, body := RegistrationVerification("alice", "482913", testCodeTTL)

	if subject == "" {
		t.Fatal("expected non-empty subject")
	}
	if !strings.Contains(body, "alice") {
		t.Errorf("expected body to contain username, got: %q", body)
	}
	if !strings.Contains(body, "482913") {
		t.Errorf("expected body to contain code, got: %q", body)
	}
}

func TestExistingAccountVerification_ContainsCode(t *testing.T) {
	subject, body := ExistingAccountVerification("777001", testCodeTTL)

	if subject == "" {
		t.Fatal("expected non-empty subject")
	}
	if !strings.Contains(body, "777001") {
		t.Errorf("expected body to contain code, got: %q", body)
	}
	// This template must communicate the account already exists — that's the
	// ADR-7 asymmetry vs RegistrationVerification.
	if !strings.Contains(body, "уже существует") {
		t.Errorf("expected body to mention an existing account, got: %q", body)
	}
}

func TestPasswordReset_ContainsUsernameAndCode(t *testing.T) {
	subject, body := PasswordReset("bob", "654321", testCodeTTL)

	if subject == "" {
		t.Fatal("expected non-empty subject")
	}
	if !strings.Contains(body, "bob") {
		t.Errorf("expected body to contain username, got: %q", body)
	}
	if !strings.Contains(body, "654321") {
		t.Errorf("expected body to contain code, got: %q", body)
	}
}

// The reset flow deliberately doesn't look up a profile just to greet the
// user, so an empty name must still render a sane letter.
func TestPasswordReset_EmptyUsernameGreetsNeutrally(t *testing.T) {
	_, body := PasswordReset("", "654321", testCodeTTL)

	if strings.Contains(body, "Привет, !") {
		t.Errorf("expected a neutral greeting for an empty username, got: %q", body)
	}
	if !strings.HasPrefix(body, "Здравствуйте!") {
		t.Errorf("expected the neutral greeting, got: %q", body)
	}
}

// The lifetime in the copy comes from EMAIL_CODE_TTL, so a letter can never
// promise a different number from the one the server enforces.
func TestTemplates_ExpiryFollowsConfiguredTTL(t *testing.T) {
	_, body := RegistrationVerification("alice", "111111", 7*time.Minute)
	if !strings.Contains(body, "7 минут") {
		t.Errorf("expected the configured TTL in the copy, got: %q", body)
	}
}

func TestExpiryPhrase_RussianNumeralAgreement(t *testing.T) {
	tests := []struct {
		name string
		ttl  time.Duration
		want string
	}{
		{"rounds sub-minute up", 30 * time.Second, "1 минуту"},
		{"one", time.Minute, "1 минуту"},
		{"few", 3 * time.Minute, "3 минуты"},
		{"many", 15 * time.Minute, "15 минут"},
		{"teens are the exception", 11 * time.Minute, "11 минут"},
		{"teens are the exception, 12", 12 * time.Minute, "12 минут"},
		{"21 follows the last digit", 21 * time.Minute, "21 минуту"},
		{"22 follows the last digit", 22 * time.Minute, "22 минуты"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expiryPhrase(tt.ttl); got != tt.want {
				t.Errorf("expiryPhrase(%v) = %q, want %q", tt.ttl, got, tt.want)
			}
		})
	}
}

func TestTemplates_DistinctSubjects(t *testing.T) {
	regSubj, _ := RegistrationVerification("alice", "111111", testCodeTTL)
	existSubj, _ := ExistingAccountVerification("111111", testCodeTTL)
	resetSubj, _ := PasswordReset("alice", "111111", testCodeTTL)

	seen := map[string]bool{}
	for _, s := range []string{regSubj, existSubj, resetSubj} {
		if seen[s] {
			t.Fatalf("expected distinct subjects across templates, got duplicate: %q", s)
		}
		seen[s] = true
	}
}
