package cron

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type RoomResolverDeps struct {
	DefaultAgentID string
	ResolveRoom    func(ctx context.Context, agentID, jobID string) (room any, roomID string, err error)
	CreateRoom     func(ctx context.Context, agentID, jobID, displayName string) (room any, err error)
	LogCreated     func(ctx context.Context, agentID, jobID string, room any)
}

func GetOrCreateCronRoom(ctx context.Context, agentID, jobID, jobName string, deps RoomResolverDeps) (any, error) {
	if deps.ResolveRoom == nil || deps.CreateRoom == nil {
		return nil, errors.New("missing room resolver")
	}
	trimmedAgent := strings.TrimSpace(agentID)
	if trimmedAgent == "" {
		trimmedAgent = strings.TrimSpace(deps.DefaultAgentID)
	}
	trimmedJob := strings.TrimSpace(jobID)
	if trimmedJob == "" {
		return nil, errors.New("jobID required")
	}

	room, roomID, err := deps.ResolveRoom(ctx, trimmedAgent, trimmedJob)
	if err != nil {
		return nil, fmt.Errorf("failed to get portal: %w", err)
	}
	if strings.TrimSpace(roomID) != "" {
		return room, nil
	}

	name := strings.TrimSpace(jobName)
	if name == "" {
		name = trimmedJob
	}
	display := fmt.Sprintf("Cron: %s", name)
	createdRoom, err := deps.CreateRoom(ctx, trimmedAgent, trimmedJob, display)
	if err != nil {
		return nil, err
	}
	if deps.LogCreated != nil {
		deps.LogCreated(ctx, trimmedAgent, trimmedJob, createdRoom)
	}
	return createdRoom, nil
}
