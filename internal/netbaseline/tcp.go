package netbaseline

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"perps-latency-benchmark/internal/bench"
	"perps-latency-benchmark/internal/netlatency"
)

type Target struct {
	Network string
	Address string
	Source  string
}

type Monitor struct {
	target     Target
	interval   time.Duration
	maxSamples int

	mu     sync.RWMutex
	values []observation
}

type observation struct {
	valueNS  int64
	source   string
	priority int
}

const (
	priorityTCPConnectFallback = 10
	priorityRequestConnect     = 50
	priorityWSHeartbeat        = 80
)

func TargetFromURL(rawURL string) (Target, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return Target{}, false
	}
	port := parsed.Port()
	if port == "" {
		switch parsed.Scheme {
		case "http", "ws":
			port = "80"
		default:
			port = "443"
		}
	}
	host := parsed.Hostname()
	if host == "" {
		return Target{}, false
	}
	return Target{
		Network: "tcp",
		Address: net.JoinHostPort(host, port),
		Source:  "tcp_connect_p10:" + parsed.Scheme + "://" + net.JoinHostPort(host, port),
	}, true
}

func NewMonitor(target Target, interval time.Duration, maxSamples int) *Monitor {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if maxSamples <= 0 {
		maxSamples = 60
	}
	return &Monitor{target: target, interval: interval, maxSamples: maxSamples}
}

func (m *Monitor) Prime(ctx context.Context) error {
	value, err := m.measure(ctx)
	if err != nil {
		return err
	}
	m.record(value, m.target.Source, priorityTCPConnectFallback)
	return nil
}

func (m *Monitor) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if value, err := m.measure(ctx); err == nil {
					m.record(value, m.target.Source, priorityTCPConnectFallback)
				}
			}
		}
	}()
}

func (m *Monitor) ObserveTrace(trace netlatency.Trace) {
	if trace.Transport != "http" || trace.ConnectNS <= 0 || trace.ConnReused {
		return
	}
	m.record(trace.ConnectNS, "request_connect", priorityRequestConnect)
}

func (m *Monitor) ObserveRTT(valueNS int64, source string) {
	if source == "" {
		source = "ws_heartbeat"
	}
	m.record(valueNS, source, priorityWSHeartbeat)
}

func (m *Monitor) Snapshot() bench.NetworkBaselineSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.values) == 0 {
		return bench.NetworkBaselineSnapshot{}
	}
	bestPriority := m.values[0].priority
	for _, value := range m.values[1:] {
		if value.priority > bestPriority {
			bestPriority = value.priority
		}
	}
	var values []int64
	var source string
	for _, value := range m.values {
		if value.priority != bestPriority {
			continue
		}
		values = append(values, value.valueNS)
		source = value.source
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	index := 0
	if len(values) >= 10 {
		index = int(0.1 * float64(len(values)-1))
	}
	return bench.NetworkBaselineSnapshot{
		FloorNS: values[index],
		Source:  source,
	}
}

func (m *Monitor) measure(ctx context.Context) (int64, error) {
	if m == nil || m.target.Address == "" {
		return 0, fmt.Errorf("network baseline target is empty")
	}
	dialer := net.Dialer{Timeout: 3 * time.Second}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, m.target.Network, m.target.Address)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return time.Since(start).Nanoseconds(), nil
}

func (m *Monitor) record(value int64, source string, priority int) {
	if value <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values = append(m.values, observation{valueNS: value, source: source, priority: priority})
	if len(m.values) > m.maxSamples {
		copy(m.values, m.values[len(m.values)-m.maxSamples:])
		m.values = m.values[:m.maxSamples]
	}
}
