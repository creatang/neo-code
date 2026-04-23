package feishu

import (
	"sync"
	"sync/atomic"
	"time"
)

// Snapshot 是适配器指标快照。
type Snapshot struct {
	RequestsTotal          uint64    `json:"requests_total"`
	RequestsRejected       uint64    `json:"requests_rejected"`
	RunsAccepted           uint64    `json:"runs_accepted"`
	RunsCompleted          uint64    `json:"runs_completed"`
	RunsFailed             uint64    `json:"runs_failed"`
	RunsCanceled           uint64    `json:"runs_canceled"`
	WatchdogTimeouts       uint64    `json:"watchdog_timeouts"`
	ConnectionReconnects   uint64    `json:"connection_reconnects"`
	AuthFailures           uint64    `json:"auth_failures"`
	QueueBackpressureDrops uint64    `json:"queue_backpressure_drops"`
	EventDuplicates        uint64    `json:"event_duplicates"`
	PermissionsPending     int       `json:"permissions_pending"`
	InflightRuns           int       `json:"inflight_runs"`
	LastGatewayPingAt      time.Time `json:"last_gateway_ping_at,omitempty"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// Metrics 提供线程安全计数器。
type Metrics struct {
	requestsTotal          atomic.Uint64
	requestsRejected       atomic.Uint64
	runsAccepted           atomic.Uint64
	runsCompleted          atomic.Uint64
	runsFailed             atomic.Uint64
	runsCanceled           atomic.Uint64
	watchdogTimeouts       atomic.Uint64
	connectionReconnects   atomic.Uint64
	authFailures           atomic.Uint64
	queueBackpressureDrops atomic.Uint64
	eventDuplicates        atomic.Uint64

	mu                 sync.Mutex
	permissionsPending int
	inflightRuns       int
	lastGatewayPingAt  time.Time
}

func (m *Metrics) IncRequestsTotal() {
	if m != nil {
		m.requestsTotal.Add(1)
	}
}
func (m *Metrics) IncRequestsRejected() {
	if m != nil {
		m.requestsRejected.Add(1)
	}
}
func (m *Metrics) IncRunsAccepted() {
	if m != nil {
		m.runsAccepted.Add(1)
	}
}
func (m *Metrics) IncRunsCompleted() {
	if m != nil {
		m.runsCompleted.Add(1)
	}
}
func (m *Metrics) IncRunsFailed() {
	if m != nil {
		m.runsFailed.Add(1)
	}
}
func (m *Metrics) IncRunsCanceled() {
	if m != nil {
		m.runsCanceled.Add(1)
	}
}
func (m *Metrics) IncWatchdogTimeouts() {
	if m != nil {
		m.watchdogTimeouts.Add(1)
	}
}
func (m *Metrics) IncConnectionReconnects() {
	if m != nil {
		m.connectionReconnects.Add(1)
	}
}
func (m *Metrics) IncAuthFailures() {
	if m != nil {
		m.authFailures.Add(1)
	}
}
func (m *Metrics) IncQueueBackpressureDrops() {
	if m != nil {
		m.queueBackpressureDrops.Add(1)
	}
}
func (m *Metrics) IncEventDuplicates() {
	if m != nil {
		m.eventDuplicates.Add(1)
	}
}

func (m *Metrics) SetPermissionsPending(count int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.permissionsPending = count
	m.mu.Unlock()
}

func (m *Metrics) SetInflightRuns(count int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.inflightRuns = count
	m.mu.Unlock()
}

func (m *Metrics) MarkGatewayPing(now time.Time) {
	if m == nil {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m.mu.Lock()
	m.lastGatewayPingAt = now.UTC()
	m.mu.Unlock()
}

// Snapshot 返回当前指标快照。
func (m *Metrics) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{UpdatedAt: time.Now().UTC()}
	}
	m.mu.Lock()
	permissionsPending := m.permissionsPending
	inflightRuns := m.inflightRuns
	lastGatewayPingAt := m.lastGatewayPingAt
	m.mu.Unlock()

	return Snapshot{
		RequestsTotal:          m.requestsTotal.Load(),
		RequestsRejected:       m.requestsRejected.Load(),
		RunsAccepted:           m.runsAccepted.Load(),
		RunsCompleted:          m.runsCompleted.Load(),
		RunsFailed:             m.runsFailed.Load(),
		RunsCanceled:           m.runsCanceled.Load(),
		WatchdogTimeouts:       m.watchdogTimeouts.Load(),
		ConnectionReconnects:   m.connectionReconnects.Load(),
		AuthFailures:           m.authFailures.Load(),
		QueueBackpressureDrops: m.queueBackpressureDrops.Load(),
		EventDuplicates:        m.eventDuplicates.Load(),
		PermissionsPending:     permissionsPending,
		InflightRuns:           inflightRuns,
		LastGatewayPingAt:      lastGatewayPingAt,
		UpdatedAt:              time.Now().UTC(),
	}
}
