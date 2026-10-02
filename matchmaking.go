package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrDeadlinePassed = errors.New("course deadline has passed")
	ErrRoomFull       = errors.New("course room is full")
)

type MatchRequest struct {
	CourseID    string    `json:"course_id"`
	LearnerID   string    `json:"learner_id"`
	DisplayName string    `json:"display_name"`
	EducatorID  string    `json:"educator_id"`
	Deadline    time.Time `json:"deadline"`
}

type MatchResult struct {
	Room          string `json:"room"`
	Channel       string `json:"channel"`
	RTCtoken      string `json:"rtc_token"`
	RealtimeToken string `json:"realtime_token"`
	SeatsUsed     int    `json:"seats_used"`
	SeatsTotal    int    `json:"seats_total"`
	ReportState   string `json:"report_state"`
}

type RoomProvisioner interface {
	CreateRoom(context.Context, string, int) error
	CreateChannel(context.Context, string) error
	PublishMatch(context.Context, string, MatchRequest, int, int) error
	IssueRTCToken(context.Context, string, MatchRequest) (string, error)
	IssueRealtimeToken(context.Context, string, MatchRequest) (string, error)
}

type Matchmaker struct {
	mu          sync.Mutex
	now         func() time.Time
	capacity    int
	provisioner RoomProvisioner
	rooms       map[string]map[string]struct{}
	ready       map[string]bool
}

func NewMatchmaker(capacity int, now func() time.Time, provisioner RoomProvisioner) *Matchmaker {
	return &Matchmaker{
		now:         now,
		capacity:    capacity,
		provisioner: provisioner,
		rooms:       make(map[string]map[string]struct{}),
		ready:       make(map[string]bool),
	}
}

func (m *Matchmaker) Match(ctx context.Context, req MatchRequest) (MatchResult, error) {
	if req.CourseID == "" || req.LearnerID == "" || req.DisplayName == "" || req.EducatorID == "" || req.Deadline.IsZero() {
		return MatchResult{}, errors.New("course_id, learner_id, display_name, educator_id, and deadline are required")
	}
	if !m.now().Before(req.Deadline) {
		return MatchResult{}, ErrDeadlinePassed
	}

	room := "course-" + slug(req.CourseID)
	channel := room + "-state"

	m.mu.Lock()
	defer m.mu.Unlock()

	members := m.rooms[room]
	if members == nil {
		members = make(map[string]struct{})
		m.rooms[room] = members
	}
	if _, matched := members[req.LearnerID]; !matched && len(members) >= m.capacity {
		return MatchResult{}, ErrRoomFull
	}
	if !m.ready[room] {
		if err := m.provisioner.CreateRoom(ctx, room, m.capacity); err != nil {
			return MatchResult{}, fmt.Errorf("create room: %w", err)
		}
		if err := m.provisioner.CreateChannel(ctx, channel); err != nil {
			return MatchResult{}, fmt.Errorf("create channel: %w", err)
		}
		m.ready[room] = true
	}

	rtcToken, err := m.provisioner.IssueRTCToken(ctx, room, req)
	if err != nil {
		return MatchResult{}, fmt.Errorf("issue rtc token: %w", err)
	}
	realtimeToken, err := m.provisioner.IssueRealtimeToken(ctx, channel, req)
	if err != nil {
		return MatchResult{}, fmt.Errorf("issue realtime token: %w", err)
	}
	_, alreadyMatched := members[req.LearnerID]
	if !alreadyMatched {
		members[req.LearnerID] = struct{}{}
	}
	if err := m.provisioner.PublishMatch(ctx, channel, req, len(members), m.capacity); err != nil {
		if !alreadyMatched {
			delete(members, req.LearnerID)
		}
		return MatchResult{}, fmt.Errorf("publish match: %w", err)
	}

	return MatchResult{
		Room: room, Channel: channel, RTCtoken: rtcToken, RealtimeToken: realtimeToken,
		SeatsUsed: len(members), SeatsTotal: m.capacity, ReportState: "learner_matched",
	}, nil
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
