// Package cachemetrics — реализация cache.Metrics на Prometheus. Вынесена в
// отдельный пакет, чтобы package cache не зависел от Prometheus.
package cachemetrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/cache"
)

type prom struct {
	hits   *prometheus.CounterVec
	misses *prometheus.CounterVec
	errors *prometheus.CounterVec
	dur    *prometheus.HistogramVec
}

// New регистрирует метрики кеша в reg и возвращает реализацию cache.Metrics.
// reg передаётся явно (а не глобальный реестр), чтобы тесты использовали свежий
// prometheus.NewRegistry() без паник двойной регистрации.
func New(reg prometheus.Registerer) cache.Metrics {
	p := &prom{
		hits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Cache hits by cache name.",
		}, []string{"cache"}),
		misses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Cache misses by cache name.",
		}, []string{"cache"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cache_errors_total",
			Help: "Cache backend errors (including timeouts) by cache name.",
		}, []string{"cache"}),
		dur: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cache_op_duration_seconds",
			Help:    "Cache operation latency by cache name and op.",
			Buckets: []float64{0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5},
		}, []string{"cache", "op"}),
	}
	reg.MustRegister(p.hits, p.misses, p.errors, p.dur)
	return p
}

func (p *prom) Hit(name string)   { p.hits.WithLabelValues(name).Inc() }
func (p *prom) Miss(name string)  { p.misses.WithLabelValues(name).Inc() }
func (p *prom) Error(name string) { p.errors.WithLabelValues(name).Inc() }

func (p *prom) ObserveLatency(name, op string, d time.Duration) {
	p.dur.WithLabelValues(name, op).Observe(d.Seconds())
}
