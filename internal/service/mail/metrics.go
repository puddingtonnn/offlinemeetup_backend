package mail

// Metrics observes outbound-mail send outcomes. Declared at the consumer
// (package mail, mirroring cache.Metrics in package cache) so the Prometheus
// implementation can live in a separate package that imports Prometheus
// (internal/service/mail/mailmetrics) without this package depending on it.
//
// The send itself happens off the request goroutine (ADR-11, see
// AuthService.sendMailAsync) — a relay failure there is otherwise silent, so
// SendFailure is the one signal ops has that ADR-11's backgrounding is
// costing real deliverability.
type Metrics interface {
	SendFailure()
}

// NopMetrics is the no-op Metrics used when no Prometheus registry is wired
// (e.g. tests, or callers that don't care about mail metrics).
var NopMetrics Metrics = nopMetrics{}

type nopMetrics struct{}

func (nopMetrics) SendFailure() {}
