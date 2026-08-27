package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/rossbrigoli/skquad/control-plane/internal/domain"
	"github.com/rossbrigoli/skquad/control-plane/internal/storage"
)

const (
	defaultOutboxBatchSize = 20
	defaultOutboxLease     = 2 * time.Minute
	defaultOutboxRetry     = 30 * time.Second
	defaultOutboxInterval  = 5 * time.Second
)

type outboxWriter interface {
	UpsertSquad(ctx context.Context, squad *domain.Squad) error
	DeleteSquad(ctx context.Context, squad *domain.Squad) error
	UpsertAgent(ctx context.Context, agent *domain.Agent, identity *domain.AgentIdentity) error
	DeleteAgent(ctx context.Context, agent *domain.Agent) error
}

// RunOutboxWorker continuously applies durable Kubernetes intents until ctx is
// cancelled. It is intentionally small; the database lease gates concurrent API
// replicas so duplicate workers do not process the same event at once.
func RunOutboxWorker(ctx context.Context, store storage.KubernetesOutboxStore, writer outboxWriter) {
	ticker := time.NewTicker(defaultOutboxInterval)
	defer ticker.Stop()
	for {
		if _, err := ProcessOutboxOnce(ctx, store, writer); err != nil && ctx.Err() == nil {
			slog.Warn("process kubernetes outbox", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessOutboxOnce leases and applies one batch of pending/failed outbox
// events. It returns the number of leased events.
func ProcessOutboxOnce(ctx context.Context, store storage.KubernetesOutboxStore, writer outboxWriter) (int, error) {
	events, err := store.LeaseKubernetesOutbox(ctx, defaultOutboxBatchSize, defaultOutboxLease)
	if err != nil {
		return 0, err
	}
	for _, event := range events {
		if err := applyOutboxEvent(ctx, writer, event); err != nil {
			markErr := store.MarkKubernetesOutboxFailed(ctx, event.ID, err.Error(), retryDelay(event.Attempts))
			if markErr != nil {
				return len(events), fmt.Errorf("mark outbox event failed: %w", markErr)
			}
			continue
		}
		if err := store.MarkKubernetesOutboxApplied(ctx, event.ID); err != nil {
			return len(events), fmt.Errorf("mark outbox event applied: %w", err)
		}
	}
	return len(events), nil
}

func applyOutboxEvent(ctx context.Context, writer outboxWriter, event *domain.KubernetesOutboxEvent) error {
	var payload domain.KubernetesOutboxPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("decode outbox payload: %w", err)
	}
	switch event.Operation {
	case domain.KubernetesOpUpsertSquad:
		if payload.Squad == nil {
			return fmt.Errorf("outbox event %s missing squad payload", event.ID)
		}
		return writer.UpsertSquad(ctx, payload.Squad)
	case domain.KubernetesOpDeleteSquad:
		if payload.Squad == nil {
			return fmt.Errorf("outbox event %s missing squad payload", event.ID)
		}
		return writer.DeleteSquad(ctx, payload.Squad)
	case domain.KubernetesOpUpsertAgent:
		if payload.Agent == nil {
			return fmt.Errorf("outbox event %s missing agent payload", event.ID)
		}
		return writer.UpsertAgent(ctx, payload.Agent, payload.Identity)
	case domain.KubernetesOpDeleteAgent:
		if payload.Agent == nil {
			return fmt.Errorf("outbox event %s missing agent payload", event.ID)
		}
		return writer.DeleteAgent(ctx, payload.Agent)
	default:
		return fmt.Errorf("unknown kubernetes outbox operation %q", event.Operation)
	}
}

func retryDelay(attempts int) time.Duration {
	delay := defaultOutboxRetry
	for i := 0; i < attempts; i++ {
		delay *= 2
		if delay >= 5*time.Minute {
			return 5 * time.Minute
		}
	}
	return delay
}
