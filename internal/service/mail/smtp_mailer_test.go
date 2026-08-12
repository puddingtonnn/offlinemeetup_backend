package mail

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A full DialAndSend round trip needs a TLS-speaking SMTP relay (smtpMailer
// requires TLSMandatory) which is disproportionate to fake for a smoke test
// (see task-7 brief's escape hatch). These tests instead cover the two parts
// of the adapter that fail before any network I/O happens: config parsing at
// construction, and address validation inside Send.

func TestNewSMTPMailer_InvalidPortReturnsError(t *testing.T) {
	_, err := NewSMTPMailer("smtp.example.com", "not-a-port", "user", "pass", "from@example.com")
	require.Error(t, err)
}

func TestNewSMTPMailer_ValidConfigSucceeds(t *testing.T) {
	m, err := NewSMTPMailer("smtp.example.com", "587", "user", "pass", "from@example.com")
	require.NoError(t, err)
	require.NotNil(t, m)
}

func TestSMTPMailer_Send_InvalidFromAddressErrorsWithoutDialing(t *testing.T) {
	m, err := NewSMTPMailer("smtp.example.com", "587", "user", "pass", "not-an-address")
	require.NoError(t, err)

	err = m.Send(context.Background(), "to@example.com", "subject", "body")
	assert.Error(t, err)
}

func TestSMTPMailer_Send_InvalidRecipientErrorsWithoutDialing(t *testing.T) {
	m, err := NewSMTPMailer("smtp.example.com", "587", "user", "pass", "from@example.com")
	require.NoError(t, err)

	err = m.Send(context.Background(), "not-an-address", "subject", "body")
	assert.Error(t, err)
}
