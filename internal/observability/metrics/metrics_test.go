package metrics

import (
	"strings"
	"sync"
	"testing"
)

func TestDefault_NoDuplicatePanics(t *testing.T) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Add(16)
	for range 16 {
		go func() {
			defer wg.Done()
			_ = Default()
		}()
	}
	wg.Wait()
}

func TestRenderPrometheus_IncludesCriticalMetrics(t *testing.T) {
	t.Helper()
	m := NewStore()
	m.IncMessagesSent("telegram")
	m.IncMessagesFailed("telegram")
	m.IncMessagesReceived("telegram")
	m.IncAdapterRetries("telegram")
	m.IncChannelReconnects("whatsapp")
	m.SetDLQDepth("telegram", 3)
	m.ObserveMessageLatency("telegram", 0.12)
	m.SetGatewayUp(true)

	out := m.RenderPrometheus()
	required := []string{
		"messages_sent_total",
		"messages_failed_total",
		"message_latency_seconds_bucket",
		"dlq_depth",
		"channel_reconnects_total",
		"adapter_retries_total",
		"gateway_health 1",
	}
	for _, token := range required {
		if !strings.Contains(out, token) {
			t.Fatalf("expected rendered metrics to contain %q", token)
		}
	}
}
