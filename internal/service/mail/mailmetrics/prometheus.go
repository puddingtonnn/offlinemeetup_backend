// Package mailmetrics is the Prometheus implementation of mail.Metrics.
// Split into its own package so internal/service/mail does not depend on
// Prometheus (same shape as internal/cache/cachemetrics for cache.Metrics).
package mailmetrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/service/mail"
)

type prom struct {
	failures prometheus.Counter
}

// New registers the mail metrics on reg and returns a mail.Metrics
// implementation. reg is passed explicitly (not the global registry) so
// tests can use a fresh prometheus.NewRegistry() without double-registration
// panics.
func New(reg prometheus.Registerer) mail.Metrics {
	p := &prom{
		failures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mail_send_failures_total",
			Help: "Outbound email sends that returned an error from Mailer.Send.",
		}),
	}
	reg.MustRegister(p.failures)
	return p
}

func (p *prom) SendFailure() { p.failures.Inc() }
