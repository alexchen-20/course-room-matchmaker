package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordingProvisioner struct {
	createdRooms int
	published    int
}

func (p *recordingProvisioner) CreateRoom(context.Context, string, int) error {
	p.createdRooms++
	return nil
}
func (p *recordingProvisioner) CreateChannel(context.Context, string) error { return nil }
func (p *recordingProvisioner) PublishMatch(context.Context, string, MatchRequest, int, int) error {
	p.published++
	return nil
}
func (p *recordingProvisioner) IssueRTCToken(context.Context, string, MatchRequest) (string, error) {
	return "rtc-test-token", nil
}
func (p *recordingProvisioner) IssueRealtimeToken(context.Context, string, MatchRequest) (string, error) {
	return "realtime-test-token", nil
}

func TestMatchDecision(t *testing.T) {
	now := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		capacity  int
		requests  []MatchRequest
		wantError error
		wantUsed  int
	}{
		{
			name: "learner before deadline gets a seat", capacity: 2,
			requests: []MatchRequest{request("learner-1", now.Add(time.Hour))}, wantUsed: 1,
		},
		{
			name: "learner at deadline is rejected", capacity: 2,
			requests: []MatchRequest{request("learner-1", now)}, wantError: ErrDeadlinePassed,
		},
		{
			name: "new learner beyond capacity is rejected", capacity: 1,
			requests:  []MatchRequest{request("learner-1", now.Add(time.Hour)), request("learner-2", now.Add(time.Hour))},
			wantError: ErrRoomFull, wantUsed: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &recordingProvisioner{}
			matcher := NewMatchmaker(tt.capacity, func() time.Time { return now }, provider)
			var result MatchResult
			var err error
			for _, req := range tt.requests {
				result, err = matcher.Match(context.Background(), req)
			}
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Match() error = %v, want %v", err, tt.wantError)
			}
			if tt.wantError == nil && result.SeatsUsed != tt.wantUsed {
				t.Fatalf("SeatsUsed = %d, want %d", result.SeatsUsed, tt.wantUsed)
			}
			if provider.published != tt.wantUsed {
				t.Fatalf("published = %d, want %d", provider.published, tt.wantUsed)
			}
			if tt.wantUsed > 0 && provider.createdRooms != 1 {
				t.Fatalf("createdRooms = %d, want 1", provider.createdRooms)
			}
		})
	}
}

func request(learner string, deadline time.Time) MatchRequest {
	return MatchRequest{
		CourseID: "algebra-2", LearnerID: learner, DisplayName: "Test Learner",
		EducatorID: "educator-7", Deadline: deadline,
	}
}
