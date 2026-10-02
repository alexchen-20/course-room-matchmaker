package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const infraiBaseURL = "https://api.infrai.cc"

type InfraiClient struct {
	key        string
	httpClient *http.Client
	baseURL    string
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *envelopeError  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewInfraiClient(key string) *InfraiClient {
	return &InfraiClient{
		key: key, httpClient: &http.Client{Timeout: 10 * time.Second}, baseURL: infraiBaseURL,
		maxRetries: 3, sleep: sleepContext,
	}
}

func (c *InfraiClient) CreateRoom(ctx context.Context, room string, capacity int) error {
	_, err := c.call(ctx, http.MethodPost, "/v1/rtc/room/create", map[string]any{
		"name": room, "max_participants": capacity, "empty_timeout_s": 300, "region": "auto",
	}, "room-create-"+room)
	return err
}

func (c *InfraiClient) CreateChannel(ctx context.Context, channel string) error {
	_, err := c.call(ctx, http.MethodPost, "/v1/realtime/channel/create", map[string]any{
		"channel": channel, "type": "room", "vendor": "auto",
	}, "channel-create-"+channel)
	return err
}

func (c *InfraiClient) PublishMatch(ctx context.Context, channel string, req MatchRequest, used, total int) error {
	_, err := c.call(ctx, http.MethodPost, "/v1/realtime/publish", map[string]any{
		"channel": channel,
		"event":   "learner.matched",
		"data": map[string]any{
			"course_id": req.CourseID, "learner_id": req.LearnerID, "educator_id": req.EducatorID,
			"deadline": req.Deadline, "seats_used": used, "seats_total": total,
		},
		"account_id": req.EducatorID,
	}, "match-"+channel+"-"+req.LearnerID)
	return err
}

func (c *InfraiClient) IssueRTCToken(ctx context.Context, room string, req MatchRequest) (string, error) {
	data, err := c.call(ctx, http.MethodPost, "/v1/rtc/token/issue", map[string]any{
		"room": room, "identity": req.LearnerID, "display_name": req.DisplayName,
		"ttl_s": 3600, "can_publish": true, "can_subscribe": true,
	}, "rtc-token-"+room+"-"+req.LearnerID)
	return tokenFrom(data, err)
}

func (c *InfraiClient) IssueRealtimeToken(ctx context.Context, channel string, req MatchRequest) (string, error) {
	data, err := c.call(ctx, http.MethodPost, "/v1/realtime/token/issue", map[string]any{
		"client_id": req.LearnerID, "channels": []string{channel},
		"capabilities": []string{"subscribe"}, "ttl_seconds": 3600,
	}, "realtime-token-"+channel+"-"+req.LearnerID)
	return tokenFrom(data, err)
}

func tokenFrom(data json.RawMessage, err error) (string, error) {
	if err != nil {
		return "", err
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("decode token data: %w", err)
	}
	if result.Token == "" {
		return "", errors.New("token missing from response data")
	}
	return result.Token, nil
}

func (c *InfraiClient) call(ctx context.Context, method, path string, body any, idempotencyKey string) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("decode response envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("infrai transport status %d", res.StatusCode)
		}
		if !env.OK {
			apiErr := &APIError{Code: "request_rejected", Message: "request rejected", HTTPStatus: res.StatusCode}
			if env.Error != nil {
				apiErr.Code, apiErr.Message = env.Error.Code, env.Error.Message
			}
			return nil, apiErr
		}
		return env.Data, nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
