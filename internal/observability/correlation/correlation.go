package correlation

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ctxKey string

const (
	requestIDKey ctxKey = "observability.request_id"
	sessionIDKey ctxKey = "observability.session_id"
	channelKey   ctxKey = "observability.channel"
	traceIDKey   ctxKey = "observability.trace_id"
)

const (
	HeaderRequestID = "X-Request-Id"
	HeaderTraceID   = "X-Trace-Id"
)

func WithRequestID(ctx context.Context, requestID string) context.Context {
	if strings.TrimSpace(requestID) == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, strings.TrimSpace(requestID))
}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	if strings.TrimSpace(sessionID) == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionIDKey, strings.TrimSpace(sessionID))
}

func WithChannel(ctx context.Context, channel string) context.Context {
	if strings.TrimSpace(channel) == "" {
		return ctx
	}
	return context.WithValue(ctx, channelKey, strings.TrimSpace(channel))
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	if strings.TrimSpace(traceID) == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey, strings.TrimSpace(traceID))
}

func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

func SessionID(ctx context.Context) string {
	v, _ := ctx.Value(sessionIDKey).(string)
	return v
}

func Channel(ctx context.Context) string {
	v, _ := ctx.Value(channelKey).(string)
	return v
}

func TraceID(ctx context.Context) string {
	v, _ := ctx.Value(traceIDKey).(string)
	return v
}

// EnsureRequestID guarantees request_id exists on ctx and returns the resulting ctx + id.
func EnsureRequestID(ctx context.Context) (context.Context, string) {
	if rid := RequestID(ctx); rid != "" {
		return ctx, rid
	}
	rid := uuid.New().String()
	return WithRequestID(ctx, rid), rid
}

// EnrichFromHTTPHeaders propagates correlation fields from headers.
func EnrichFromHTTPHeaders(ctx context.Context, headers http.Header) context.Context {
	if rid := strings.TrimSpace(headers.Get(HeaderRequestID)); rid != "" {
		ctx = WithRequestID(ctx, rid)
	}
	if tid := strings.TrimSpace(headers.Get(HeaderTraceID)); tid != "" {
		ctx = WithTraceID(ctx, tid)
	}
	// Accept traceparent and extract the 32 hex trace-id from version-traceid-parentid-flags.
	if tp := strings.TrimSpace(headers.Get("traceparent")); tp != "" {
		parts := strings.Split(tp, "-")
		if len(parts) >= 2 && len(parts[1]) == 32 {
			ctx = WithTraceID(ctx, parts[1])
		}
	}
	return ctx
}

func Fields(ctx context.Context) []zap.Field {
	fields := make([]zap.Field, 0, 4)
	if rid := RequestID(ctx); rid != "" {
		fields = append(fields, zap.String("request_id", rid))
	}
	if sid := SessionID(ctx); sid != "" {
		fields = append(fields, zap.String("session_id", sid))
	}
	if ch := Channel(ctx); ch != "" {
		fields = append(fields, zap.String("channel", ch))
	}
	if tid := TraceID(ctx); tid != "" {
		fields = append(fields, zap.String("trace_id", tid))
	}
	return fields
}
