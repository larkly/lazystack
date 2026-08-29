package cloud

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/shared"
)

const (
	retryMaxRetries    = 3                      // retries after the initial attempt (4 attempts total)
	retryBaseBackoff   = 250 * time.Millisecond // first backoff, doubled per attempt
	retryMaxBackoff    = 2 * time.Second        // backoff cap before jitter
	retryMaxRetryAfter = 15 * time.Second       // cap on a server-provided Retry-After
	retryMaxBodySize   = 1 << 20                // 1MB — largest request body we buffer for replay
)

// newHTTPClient builds an http.Client with transport-level timeouts and
// retry-on-429/5xx behavior. The TLS config, when non-nil, is applied to the
// transport here (not via gophercloud's config.WithTLSConfig, which replaces
// the transport of any client passed with config.WithHTTPClient).
func newHTTPClient(tlsConfig *tls.Config) http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig
	}
	transport.DialContext = (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = 1 * time.Second
	transport.IdleConnTimeout = 90 * time.Second
	transport.MaxIdleConnsPerHost = 10
	return http.Client{Transport: &retryTransport{Base: transport}}
}

// retryTransport is an http.RoundTripper that retries requests receiving
// 429/502/503/504 responses, honoring the Retry-After header when present
// (capped) and falling back to jittered exponential backoff otherwise.
// Requests with a replayable body are retried with a fresh body reader per
// attempt; unbufferable bodies larger than retryMaxBodySize are sent once.
type retryTransport struct {
	Base http.RoundTripper
}

// RoundTrip executes the request, retrying up to retryMaxRetries times on
// retryable status codes. The request is left usable after RoundTrip returns.
// Non-idempotent methods (POST, PATCH) are only retried on 429: a throttled
// request is known not to have executed, while a 5xx on a mutating call may
// have been processed before the gateway timed out, and a retry could create
// a duplicate resource.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	replayable := replayableBody(req)

	for attempt := 0; ; attempt++ {
		resp, err := base.RoundTrip(req)
		if err != nil {
			restoreBody(req)
			return resp, err
		}
		if !isRetryableStatus(resp.StatusCode, req.Method) || !replayable || attempt >= retryMaxRetries {
			restoreBody(req)
			return resp, nil
		}

		// Drain and close so the connection can be reused.
		drainAndClose(resp.Body)
		shared.Debugf("[cloud] retryTransport: %s %s -> %d (attempt %d/%d), retrying",
			req.Method, req.URL, resp.StatusCode, attempt+1, retryMaxRetries)

		if werr := retryWait(req.Context(), attempt, resp.Header.Get("Retry-After")); werr != nil {
			return nil, werr
		}
		if !restoreBody(req) {
			// Body could not be replayed; surface the last response rather
			// than sending a request with an empty body.
			return resp, nil
		}
	}
}

// isRetryableStatus reports whether the status code should trigger a retry
// for the given method. 429 is always retried (the request was throttled
// before execution); 502/503/504 are only retried for idempotent methods.
func isRetryableStatus(status int, method string) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if method != http.MethodPost && method != http.MethodPatch {
		return status == http.StatusBadGateway ||
			status == http.StatusServiceUnavailable ||
			status == http.StatusGatewayTimeout
	}
	return false
}

// replayableBody makes the request body replayable across attempts. Bodies
// with an existing GetBody are already replayable. Otherwise the body is
// buffered into memory when it fits within retryMaxBodySize; larger or
// unreadable bodies are reassembled for a single (unretried) attempt.
func replayableBody(req *http.Request) bool {
	if req.Body == nil || req.GetBody != nil {
		// No body to replay, or already replayable.
		return true
	}
	if req.ContentLength > retryMaxBodySize {
		return false
	}

	buf, err := io.ReadAll(io.LimitReader(req.Body, retryMaxBodySize+1))
	if err != nil || int64(len(buf)) > retryMaxBodySize {
		// Unbufferable: splice back what we consumed so the request can
		// still proceed, but mark it non-retryable.
		req.Body = readCloser{
			Reader: io.MultiReader(bytes.NewReader(buf), req.Body),
			Closer: req.Body,
		}
		return false
	}

	req.Body.Close()
	body := buf
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	restoreBody(req)
	req.ContentLength = int64(len(body))
	return true
}

// restoreBody puts a fresh body reader back into the request, leaving it
// usable for the next attempt (or by the caller after RoundTrip returns).
// It reports whether the body was successfully restored.
func restoreBody(req *http.Request) bool {
	if req.Body != nil && req.GetBody != nil {
		if fresh, err := req.GetBody(); err == nil {
			req.Body = fresh
			return true
		}
		return false
	}
	return true
}

type readCloser struct {
	io.Reader
	io.Closer
}

// retryWait sleeps until the next attempt: the server-provided Retry-After
// (seconds or HTTP-date, capped at retryMaxRetryAfter) or a jittered
// exponential backoff. It returns early if the request context is cancelled.
func retryWait(ctx context.Context, attempt int, retryAfter string) error {
	timer := time.NewTimer(retryDelay(attempt, retryAfter))
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// retryDelay computes the wait before the next attempt.
func retryDelay(attempt int, retryAfter string) time.Duration {
	if v := strings.TrimSpace(retryAfter); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			return min(time.Duration(secs)*time.Second, retryMaxRetryAfter)
		}
		if t, err := http.ParseTime(v); err == nil {
			d := time.Until(t)
			if d < 0 {
				d = 0
			}
			return min(d, retryMaxRetryAfter)
		}
	}
	d := retryBaseBackoff << attempt
	return min(d, retryMaxBackoff) + time.Duration(rand.IntN(101))*time.Millisecond
}

// drainAndClose discards the remaining response body (bounded, so a
// misbehaving server streaming an endless error body cannot stall the
// retry loop) and closes it.
func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.CopyN(io.Discard, body, drainLimit)
	body.Close()
}

// drainLimit bounds how much of an error body is drained before closing.
const drainLimit = 64 << 10
