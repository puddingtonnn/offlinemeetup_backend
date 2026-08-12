package mailmetrics

import (
	"testing"

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

func TestPrometheus_RecordsSendFailures(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.SendFailure()
	m.SendFailure()

	fams, err := reg.Gather()
	require.NoError(t, err)

	fam := findFamily(fams, "mail_send_failures_total")
	require.NotNil(t, fam)
	require.Len(t, fam.GetMetric(), 1)
	assert.Equal(t, float64(2), fam.GetMetric()[0].GetCounter().GetValue())
}
