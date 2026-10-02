package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestClientDecodesRejectionBeforeStatus(t *testing.T) {
	client := NewInfraiClient("test-key")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":false,"data":null,"error":{"code":"COURSE_INPUT_REJECTED","message":"course input rejected"},"metadata":{}}`)),
		}, nil
	})}
	_, err := client.call(context.Background(), http.MethodPost, "/v1/realtime/publish", map[string]any{
		"channel": "course-algebra-2-state", "event": "test", "data": map[string]any{}, "account_id": "educator-7",
	}, "request-1")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "COURSE_INPUT_REJECTED" || apiErr.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("error = %#v, want decoded APIError", err)
	}
}

func TestClientRetriesRateLimit(t *testing.T) {
	attempts := 0
	client := NewInfraiClient("test-key")
	client.sleep = func(context.Context, time.Duration) error { return nil }
	client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		status, body := http.StatusTooManyRequests, `{"ok":false,"error":{"code":"RATE_LIMITED","message":"retry later"},"metadata":{}}`
		if attempts == 2 {
			status, body = http.StatusOK, `{"ok":true,"data":{"accepted":true},"error":null,"metadata":{}}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"1"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	_, err := client.call(context.Background(), http.MethodPost, "/v1/realtime/publish", map[string]any{
		"channel": "course-algebra-2-state", "event": "test", "data": map[string]any{}, "account_id": "educator-7",
	}, "request-1")
	if err != nil || attempts != 2 {
		t.Fatalf("attempts = %d, error = %v", attempts, err)
	}
}
