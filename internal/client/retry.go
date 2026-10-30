package client

import (
	"net/http"
	"strconv"
	"time"
)

const (
	MAX_RATE_LIMIT_RETRIES = 4
	MAX_RETRY_WAIT         = 60 * time.Second
	DEFAULT_RETRY_WAIT     = 2 * time.Second
)

// rateLimitTransport retries requests answered 429, waiting as long as Retry-After asks.
type rateLimitTransport struct {
	next http.RoundTripper
}

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := t.next.RoundTrip(req)
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt == MAX_RATE_LIMIT_RETRIES {
			return resp, err
		}

		retryReq, ok := rewind(req)
		if !ok {
			return resp, nil
		}
		resp.Body.Close()

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(retryWait(resp)):
		}
		req = retryReq
	}
}

func rewind(req *http.Request) (*http.Request, bool) {
	if req.Body == nil || req.Body == http.NoBody {
		return req, true
	}
	if req.GetBody == nil {
		return nil, false
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, false
	}
	clone := req.Clone(req.Context())
	clone.Body = body
	return clone, true
}

func retryWait(resp *http.Response) time.Duration {
	seconds, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || seconds <= 0 {
		return DEFAULT_RETRY_WAIT
	}
	return min(time.Duration(seconds)*time.Second, MAX_RETRY_WAIT)
}
