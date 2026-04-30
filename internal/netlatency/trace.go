package netlatency

import (
	"crypto/tls"
	"net/http/httptrace"
	"sync"
	"time"
)

type Trace struct {
	StartedAt        time.Time `json:"started_at"`
	TotalNS          int64     `json:"total_ns"`
	Transport        string    `json:"transport,omitempty"`
	GetConnNS        int64     `json:"get_conn_ns,omitempty"`
	DNSNS            int64     `json:"dns_ns,omitempty"`
	ConnectNS        int64     `json:"connect_ns,omitempty"`
	TLSNS            int64     `json:"tls_ns,omitempty"`
	RequestWriteNS   int64     `json:"request_write_ns,omitempty"`
	TTFBNS           int64     `json:"ttfb_ns,omitempty"`
	ResponseReadNS   int64     `json:"response_read_ns,omitempty"`
	ResponseWaitNS   int64     `json:"response_wait_ns,omitempty"`
	ConnReused       bool      `json:"conn_reused"`
	ConnWasIdle      bool      `json:"conn_was_idle"`
	ConnIdleTimeNS   int64     `json:"conn_idle_time_ns,omitempty"`
	GotConnAtNS      int64     `json:"got_conn_at_ns,omitempty"`
	WroteRequestAtNS int64     `json:"wrote_request_at_ns,omitempty"`
	FirstByteAtNS    int64     `json:"first_byte_at_ns,omitempty"`
}

type traceRecorder struct {
	mu sync.Mutex

	start             time.Time
	getConn           time.Time
	gotConn           time.Time
	dnsStart          time.Time
	dnsDone           time.Time
	connectStart      time.Time
	connectDone       time.Time
	tlsStart          time.Time
	tlsDone           time.Time
	wroteHeaders      time.Time
	wroteRequest      time.Time
	firstResponseByte time.Time
	bodyReadDone      time.Time
	finish            time.Time

	connReused     bool
	connWasIdle    bool
	connIdleTimeNS int64
}

func newTraceRecorder(start time.Time) *traceRecorder {
	return &traceRecorder{start: start}
}

func (r *traceRecorder) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(_ string) {
			r.setTime(&r.getConn)
		},
		GotConn: func(info httptrace.GotConnInfo) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.gotConn = time.Now()
			r.connReused = info.Reused
			r.connWasIdle = info.WasIdle
			r.connIdleTimeNS = info.IdleTime.Nanoseconds()
		},
		DNSStart: func(_ httptrace.DNSStartInfo) {
			r.setTime(&r.dnsStart)
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			r.setTime(&r.dnsDone)
		},
		ConnectStart: func(_, _ string) {
			r.setTime(&r.connectStart)
		},
		ConnectDone: func(_, _ string, _ error) {
			r.setTime(&r.connectDone)
		},
		TLSHandshakeStart: func() {
			r.setTime(&r.tlsStart)
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			r.setTime(&r.tlsDone)
		},
		WroteHeaders: func() {
			r.setTime(&r.wroteHeaders)
		},
		WroteRequest: func(_ httptrace.WroteRequestInfo) {
			r.setTime(&r.wroteRequest)
		},
		GotFirstResponseByte: func() {
			r.setTime(&r.firstResponseByte)
		},
	}
}

func (r *traceRecorder) setTime(target *time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	*target = time.Now()
}

func (r *traceRecorder) markBodyReadDone() {
	r.setTime(&r.bodyReadDone)
}

func (r *traceRecorder) markFinish() {
	r.setTime(&r.finish)
}

func (r *traceRecorder) snapshot() Trace {
	r.mu.Lock()
	defer r.mu.Unlock()

	finish := r.finish
	if finish.IsZero() {
		finish = time.Now()
	}

	return Trace{
		StartedAt:        r.start.UTC(),
		TotalNS:          finish.Sub(r.start).Nanoseconds(),
		Transport:        "http",
		GetConnNS:        durationNS(r.getConn, r.gotConn),
		DNSNS:            durationNS(r.dnsStart, r.dnsDone),
		ConnectNS:        durationNS(r.connectStart, r.connectDone),
		TLSNS:            durationNS(r.tlsStart, r.tlsDone),
		RequestWriteNS:   durationNS(nonZero(r.wroteHeaders, r.gotConn), r.wroteRequest),
		TTFBNS:           durationNS(r.start, r.firstResponseByte),
		ResponseReadNS:   durationNS(r.firstResponseByte, r.bodyReadDone),
		ResponseWaitNS:   durationNS(r.wroteRequest, r.firstResponseByte),
		ConnReused:       r.connReused,
		ConnWasIdle:      r.connWasIdle,
		ConnIdleTimeNS:   r.connIdleTimeNS,
		GotConnAtNS:      sinceStartNS(r.start, r.gotConn),
		WroteRequestAtNS: sinceStartNS(r.start, r.wroteRequest),
		FirstByteAtNS:    sinceStartNS(r.start, r.firstResponseByte),
	}
}

func durationNS(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Nanoseconds()
}

func sinceStartNS(start, event time.Time) int64 {
	return durationNS(start, event)
}

func nonZero(preferred, fallback time.Time) time.Time {
	if !preferred.IsZero() {
		return preferred
	}
	return fallback
}
