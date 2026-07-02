package app

import (
	"crypto/tls"
	"fmt"
	"net/http/httptrace"
	"os"
	"strings"
	"sync"
	"time"

	"ccLoad/internal/util"

	"github.com/gin-gonic/gin"
)

const proxyTimingStartContextKey = "ccLoad.proxyTimingStart"

func proxyTimingEnabled() bool {
	return util.ParseBoolDefault(os.Getenv("CCLOAD_TIMING_TRACE"), false)
}

func captureProxyTimingStart() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(proxyTimingStartContextKey, time.Now())
		c.Next()
	}
}

func proxyTimingStartTime(c *gin.Context, fallback time.Time) time.Time {
	if c == nil {
		return fallback
	}
	if v, ok := c.Get(proxyTimingStartContextKey); ok {
		if t, ok := v.(time.Time); ok && !t.IsZero() {
			return t
		}
	}
	return fallback
}

// proxyTimingTrace captures a per-attempt timing timeline.
//
// It is intentionally enabled by env var only (CCLOAD_TIMING_TRACE=1), because
// httptrace callbacks add small overhead and the resulting log messages are
// verbose.  When disabled, callers keep a nil pointer and the hot path stays
// unchanged.
type proxyTimingTrace struct {
	mu sync.Mutex

	requestStart time.Time
	handlerStart time.Time
	queueWait    time.Duration

	attemptStart time.Time
	channelID    int64
	baseURL      string

	roundTripStart time.Time
	getConn        time.Time
	gotConn        time.Time
	gotConnReused  bool
	gotConnWasIdle bool
	gotConnIdle    time.Duration

	dnsStart time.Time
	dnsDone  time.Time
	dnsErr   string

	connectStart time.Time
	connectDone  time.Time
	connectErr   string

	tlsStart time.Time
	tlsDone  time.Time
	tlsErr   string

	wroteRequest time.Time
	wroteErr     string

	firstResponseByte time.Time
	responseHeaders   time.Time

	firstStreamEvent time.Time
	firstTextToken   time.Time
	firstClientWrite time.Time
}

func newProxyTimingTrace(requestStart, handlerStart time.Time) *proxyTimingTrace {
	if !proxyTimingEnabled() {
		return nil
	}
	return &proxyTimingTrace{
		requestStart: requestStart,
		handlerStart: handlerStart,
	}
}

func (t *proxyTimingTrace) MarkQueueWait(d time.Duration) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.queueWait = d
	t.mu.Unlock()
}

func (t *proxyTimingTrace) StartAttempt(at time.Time, channelID int64, baseURL string) {
	if t == nil {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}
	t.mu.Lock()
	t.attemptStart = at
	t.channelID = channelID
	t.baseURL = baseURL
	t.resetRoundTripLocked()
	t.mu.Unlock()
}

func (t *proxyTimingTrace) StartRoundTrip() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.resetRoundTripLocked()
	t.roundTripStart = time.Now()
	t.mu.Unlock()
}

func (t *proxyTimingTrace) resetRoundTripLocked() {
	t.roundTripStart = time.Time{}
	t.getConn = time.Time{}
	t.gotConn = time.Time{}
	t.gotConnReused = false
	t.gotConnWasIdle = false
	t.gotConnIdle = 0
	t.dnsStart = time.Time{}
	t.dnsDone = time.Time{}
	t.dnsErr = ""
	t.connectStart = time.Time{}
	t.connectDone = time.Time{}
	t.connectErr = ""
	t.tlsStart = time.Time{}
	t.tlsDone = time.Time{}
	t.tlsErr = ""
	t.wroteRequest = time.Time{}
	t.wroteErr = ""
	t.firstResponseByte = time.Time{}
	t.responseHeaders = time.Time{}
	t.firstStreamEvent = time.Time{}
	t.firstTextToken = time.Time{}
	t.firstClientWrite = time.Time{}
}

func (t *proxyTimingTrace) ClientTrace() *httptrace.ClientTrace {
	if t == nil {
		return nil
	}
	return &httptrace.ClientTrace{
		GetConn: func(_ string) {
			t.markTime(func() { t.getConn = time.Now() })
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.markTime(func() {
				t.gotConn = time.Now()
				t.gotConnReused = info.Reused
				t.gotConnWasIdle = info.WasIdle
				t.gotConnIdle = info.IdleTime
			})
		},
		DNSStart: func(_ httptrace.DNSStartInfo) {
			t.markTime(func() { t.dnsStart = time.Now() })
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			t.markTime(func() {
				t.dnsDone = time.Now()
				if info.Err != nil {
					t.dnsErr = info.Err.Error()
				}
			})
		},
		ConnectStart: func(_, _ string) {
			t.markTime(func() { t.connectStart = time.Now() })
		},
		ConnectDone: func(_, _ string, err error) {
			t.markTime(func() {
				t.connectDone = time.Now()
				if err != nil {
					t.connectErr = err.Error()
				}
			})
		},
		TLSHandshakeStart: func() {
			t.markTime(func() { t.tlsStart = time.Now() })
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			t.markTime(func() {
				t.tlsDone = time.Now()
				if err != nil {
					t.tlsErr = err.Error()
				}
			})
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			t.markTime(func() {
				t.wroteRequest = time.Now()
				if info.Err != nil {
					t.wroteErr = info.Err.Error()
				}
			})
		},
		GotFirstResponseByte: func() {
			t.markTime(func() { t.firstResponseByte = time.Now() })
		},
	}
}

func (t *proxyTimingTrace) MarkResponseHeaders() {
	if t == nil {
		return
	}
	t.markTime(func() {
		if t.responseHeaders.IsZero() {
			t.responseHeaders = time.Now()
		}
	})
}

func (t *proxyTimingTrace) MarkFirstStreamEvent() {
	if t == nil {
		return
	}
	t.markTime(func() {
		if t.firstStreamEvent.IsZero() {
			t.firstStreamEvent = time.Now()
		}
	})
}

func (t *proxyTimingTrace) MarkFirstTextToken() {
	if t == nil {
		return
	}
	t.markTime(func() {
		if t.firstTextToken.IsZero() {
			t.firstTextToken = time.Now()
		}
	})
}

func (t *proxyTimingTrace) MarkFirstClientWrite() {
	if t == nil {
		return
	}
	t.markTime(func() {
		if t.firstClientWrite.IsZero() {
			t.firstClientWrite = time.Now()
		}
	})
}

func (t *proxyTimingTrace) markTime(fn func()) {
	t.mu.Lock()
	fn()
	t.mu.Unlock()
}

func (t *proxyTimingTrace) Summary() string {
	if t == nil {
		return ""
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.attemptStart.IsZero() {
		return ""
	}

	parts := make([]string, 0, 16)
	parts = append(parts, "timing")
	if !t.requestStart.IsZero() {
		parts = append(parts, "pre_attempt="+fmtDuration(t.attemptStart.Sub(t.requestStart)))
		if !t.handlerStart.IsZero() {
			parts = append(parts, "pre_handler="+fmtDuration(t.handlerStart.Sub(t.requestStart)))
		}
	}
	if t.queueWait > 0 {
		parts = append(parts, "queue="+fmtDuration(t.queueWait))
	}
	if t.gotConnReused {
		parts = append(parts, "conn=reused")
		if t.gotConnWasIdle {
			parts = append(parts, "idle="+fmtDuration(t.gotConnIdle))
		}
	} else {
		if !t.dnsStart.IsZero() && !t.dnsDone.IsZero() {
			parts = append(parts, "dns="+fmtDuration(t.dnsDone.Sub(t.dnsStart)))
		} else if !t.dnsStart.IsZero() {
			parts = append(parts, "dns=pending")
		}
		if !t.connectStart.IsZero() && !t.connectDone.IsZero() {
			parts = append(parts, "tcp="+fmtDuration(t.connectDone.Sub(t.connectStart)))
		} else if !t.connectStart.IsZero() {
			parts = append(parts, "tcp=pending")
		}
		if !t.tlsStart.IsZero() && !t.tlsDone.IsZero() {
			parts = append(parts, "tls="+fmtDuration(t.tlsDone.Sub(t.tlsStart)))
		} else if !t.tlsStart.IsZero() {
			parts = append(parts, "tls=pending")
		}
	}
	if !t.gotConn.IsZero() {
		parts = append(parts, "got_conn_at="+fmtSince(t.attemptStart, t.gotConn))
	}
	if !t.wroteRequest.IsZero() {
		parts = append(parts, "wrote_at="+fmtSince(t.attemptStart, t.wroteRequest))
	}
	if !t.firstResponseByte.IsZero() {
		parts = append(parts, "resp_byte_at="+fmtSince(t.attemptStart, t.firstResponseByte))
	}
	if !t.responseHeaders.IsZero() {
		parts = append(parts, "headers_at="+fmtSince(t.attemptStart, t.responseHeaders))
	}
	if !t.firstStreamEvent.IsZero() {
		parts = append(parts, "first_event_at="+fmtSince(t.attemptStart, t.firstStreamEvent))
	}
	if !t.firstTextToken.IsZero() {
		parts = append(parts, "first_text_at="+fmtSince(t.attemptStart, t.firstTextToken))
		if !t.firstStreamEvent.IsZero() {
			parts = append(parts, "text_gap="+fmtDuration(t.firstTextToken.Sub(t.firstStreamEvent)))
		}
	}
	if !t.firstClientWrite.IsZero() {
		parts = append(parts, "client_write_at="+fmtSince(t.attemptStart, t.firstClientWrite))
	}
	if t.dnsErr != "" {
		parts = append(parts, "dns_err="+compactTimingErr(t.dnsErr))
	}
	if t.connectErr != "" {
		parts = append(parts, "tcp_err="+compactTimingErr(t.connectErr))
	}
	if t.tlsErr != "" {
		parts = append(parts, "tls_err="+compactTimingErr(t.tlsErr))
	}
	if t.wroteErr != "" {
		parts = append(parts, "write_err="+compactTimingErr(t.wroteErr))
	}
	return strings.Join(parts, " ")
}

func fmtSince(base, t time.Time) string {
	if base.IsZero() || t.IsZero() {
		return "-"
	}
	return fmtDuration(t.Sub(base))
}

func fmtDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func compactTimingErr(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 96 {
		return s[:96]
	}
	return s
}
