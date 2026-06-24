package cachemetrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findFamily(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}

// counterFor возвращает значение счётчика с меткой cache=value, или -1.
func counterFor(f *dto.MetricFamily, value string) float64 {
	if f == nil {
		return -1
	}
	for _, m := range f.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "cache" && l.GetValue() == value {
				return m.GetCounter().GetValue()
			}
		}
	}
	return -1
}

func TestPrometheus_RecordsLabeledMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.Hit("chats")
	m.Hit("chats")
	m.Miss("tags")
	m.Error("meetup")
	m.ObserveLatency("profile", "get", 2*time.Millisecond)

	fams, err := reg.Gather()
	require.NoError(t, err)

	assert.Equal(t, float64(2), counterFor(findFamily(fams, "cache_hits_total"), "chats"))
	assert.Equal(t, float64(1), counterFor(findFamily(fams, "cache_misses_total"), "tags"))
	assert.Equal(t, float64(1), counterFor(findFamily(fams, "cache_errors_total"), "meetup"))

	durFam := findFamily(fams, "cache_op_duration_seconds")
	require.NotNil(t, durFam)
	require.Len(t, durFam.GetMetric(), 1)
	assert.Equal(t, uint64(1), durFam.GetMetric()[0].GetHistogram().GetSampleCount())
}
