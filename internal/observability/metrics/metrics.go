package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

var histogramBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type latencyHistogram struct {
	count   uint64
	sum     float64
	buckets []uint64
}

// Store is a lightweight Prometheus-compatible in-process metric store.
// Labels are intentionally constrained to low-cardinality dimensions only.
type Store struct {
	mu sync.RWMutex

	messagesSentTotal      map[string]uint64
	messagesFailedTotal    map[string]uint64
	messagesReceivedTotal  map[string]uint64
	adapterRetriesTotal    map[string]uint64
	channelReconnectsTotal map[string]uint64
	dlqDepth               map[string]float64
	messageLatency         map[string]*latencyHistogram
	gatewayUp              float64
	preflightFailuresTotal uint64
}

func NewStore() *Store {
	return &Store{
		messagesSentTotal:      make(map[string]uint64),
		messagesFailedTotal:    make(map[string]uint64),
		messagesReceivedTotal:  make(map[string]uint64),
		adapterRetriesTotal:    make(map[string]uint64),
		channelReconnectsTotal: make(map[string]uint64),
		dlqDepth:               make(map[string]float64),
		messageLatency:         make(map[string]*latencyHistogram),
	}
}

func (s *Store) IncMessagesSent(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messagesSentTotal[channel]++
}

func (s *Store) IncMessagesFailed(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messagesFailedTotal[channel]++
}

func (s *Store) IncMessagesReceived(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messagesReceivedTotal[channel]++
}

func (s *Store) IncAdapterRetries(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adapterRetriesTotal[channel]++
}

func (s *Store) IncChannelReconnects(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelReconnectsTotal[channel]++
}

func (s *Store) ObserveMessageLatency(channel string, seconds float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.messageLatency[channel]
	if !ok {
		h = &latencyHistogram{buckets: make([]uint64, len(histogramBuckets))}
		s.messageLatency[channel] = h
	}
	h.count++
	h.sum += seconds
	for i, b := range histogramBuckets {
		if seconds <= b {
			h.buckets[i]++
		}
	}
}

func (s *Store) SetDLQDepth(channel string, depth float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dlqDepth[channel] = depth
}

func (s *Store) SetGatewayUp(up bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if up {
		s.gatewayUp = 1
		return
	}
	s.gatewayUp = 0
}

// IncPreflightFailures increments when CLI preflight (doctor subset) fails before start.
func (s *Store) IncPreflightFailures() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preflightFailuresTotal++
}

func (s *Store) RenderPrometheus() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var b strings.Builder

	b.WriteString("# HELP messages_sent_total Total outbound messages sent successfully.\n")
	b.WriteString("# TYPE messages_sent_total counter\n")
	writeCounterByChannel(&b, "messages_sent_total", s.messagesSentTotal)

	b.WriteString("# HELP messages_failed_total Total outbound messages that failed.\n")
	b.WriteString("# TYPE messages_failed_total counter\n")
	writeCounterByChannel(&b, "messages_failed_total", s.messagesFailedTotal)

	b.WriteString("# HELP messages_received_total Total inbound messages received.\n")
	b.WriteString("# TYPE messages_received_total counter\n")
	writeCounterByChannel(&b, "messages_received_total", s.messagesReceivedTotal)

	b.WriteString("# HELP adapter_retries_total Total adapter retry attempts.\n")
	b.WriteString("# TYPE adapter_retries_total counter\n")
	writeCounterByChannel(&b, "adapter_retries_total", s.adapterRetriesTotal)

	b.WriteString("# HELP channel_reconnects_total Total reconnect events by channel.\n")
	b.WriteString("# TYPE channel_reconnects_total counter\n")
	writeCounterByChannel(&b, "channel_reconnects_total", s.channelReconnectsTotal)

	b.WriteString("# HELP dlq_depth Dead-letter queue depth by channel.\n")
	b.WriteString("# TYPE dlq_depth gauge\n")
	writeGaugeByChannel(&b, "dlq_depth", s.dlqDepth)

	b.WriteString("# HELP gateway_health Gateway health indicator (1=up,0=down).\n")
	b.WriteString("# TYPE gateway_health gauge\n")
	fmt.Fprintf(&b, "gateway_health %g\n", s.gatewayUp)

	b.WriteString("# HELP preflight_failures_total Total preflight failures before daemon/gateway start.\n")
	b.WriteString("# TYPE preflight_failures_total counter\n")
	fmt.Fprintf(&b, "preflight_failures_total %d\n", s.preflightFailuresTotal)

	b.WriteString("# HELP message_latency_seconds Outbound message latency in seconds.\n")
	b.WriteString("# TYPE message_latency_seconds histogram\n")
	channels := sortedLatencyChannels(s.messageLatency)
	for _, channel := range channels {
		h := s.messageLatency[channel]
		for i, upper := range histogramBuckets {
			fmt.Fprintf(&b, "message_latency_seconds_bucket{channel=%q,le=%q} %d\n", channel, trimFloat(upper), h.buckets[i])
		}
		fmt.Fprintf(&b, "message_latency_seconds_bucket{channel=%q,le=\"+Inf\"} %d\n", channel, h.count)
		fmt.Fprintf(&b, "message_latency_seconds_sum{channel=%q} %g\n", channel, h.sum)
		fmt.Fprintf(&b, "message_latency_seconds_count{channel=%q} %d\n", channel, h.count)
	}

	return b.String()
}

func writeCounterByChannel(b *strings.Builder, name string, values map[string]uint64) {
	channels := sortedCounterKeys(values)
	for _, ch := range channels {
		fmt.Fprintf(b, "%s{channel=%q} %d\n", name, ch, values[ch])
	}
}

func writeGaugeByChannel(b *strings.Builder, name string, values map[string]float64) {
	channels := sortedGaugeKeys(values)
	for _, ch := range channels {
		fmt.Fprintf(b, "%s{channel=%q} %g\n", name, ch, values[ch])
	}
}

func sortedCounterKeys(m map[string]uint64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedGaugeKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedLatencyChannels(m map[string]*latencyHistogram) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func trimFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
}

func (s *Store) ResetForTests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messagesSentTotal = make(map[string]uint64)
	s.messagesFailedTotal = make(map[string]uint64)
	s.messagesReceivedTotal = make(map[string]uint64)
	s.adapterRetriesTotal = make(map[string]uint64)
	s.channelReconnectsTotal = make(map[string]uint64)
	s.dlqDepth = make(map[string]float64)
	s.messageLatency = make(map[string]*latencyHistogram)
	s.gatewayUp = 0
	s.preflightFailuresTotal = 0
}

var (
	defaultOnce sync.Once
	defaultInst *Store
)

func Default() *Store {
	defaultOnce.Do(func() {
		defaultInst = NewStore()
	})
	return defaultInst
}
