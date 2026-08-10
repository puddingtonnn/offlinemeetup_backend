package mail

import (
	"strings"
	"testing"
)

func TestRegistrationVerification_ContainsUsernameAndCode(t *testing.T) {
	subject, body := RegistrationVerification("alice", "482913")

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
	subject, body := ExistingAccountVerification("777001")

	if subject == "" {
		t.Fatal("expected non-empty subject")
	}
	if !strings.Contains(body, "777001") {
		t.Errorf("expected body to contain code, got: %q", body)
	}
	// This template must communicate the account already exists — that's the
	// ADR-7 asymmetry vs RegistrationVerification.
	if !strings.Contains(strings.ToLower(body), "already") {
		t.Errorf("expected body to mention an existing account, got: %q", body)
	}
}

func TestPasswordReset_ContainsUsernameAndCode(t *testing.T) {
	subject, body := PasswordReset("bob", "654321")

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

func TestTemplates_DistinctSubjects(t *testing.T) {
	regSubj, _ := RegistrationVerification("alice", "111111")
	existSubj, _ := ExistingAccountVerification("111111")
	resetSubj, _ := PasswordReset("alice", "111111")

	seen := map[string]bool{}
	for _, s := range []string{regSubj, existSubj, resetSubj} {
		if seen[s] {
			t.Fatalf("expected distinct subjects across templates, got duplicate: %q", s)
		}
		seen[s] = true
	}
}
