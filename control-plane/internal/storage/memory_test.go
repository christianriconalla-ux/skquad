package storage

import (
	"context"
	"testing"
	"time"

	"github.com/rossbrigoli/skquad/control-plane/internal/domain"
)

func TestMemoryStoreDefaultsTrustLabelsAndRawContent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore()
	squad := mustCreateMemoryTestSquad(t, ctx, store)
	agent := mustCreateMemoryTestAgent(t, ctx, store, squad.ID)

	created, err := store.CreateAgentMemory(ctx, &domain.AgentMemory{
		AgentID: agent.ID,
		SquadID: squad.ID,
		Content: "raw completion summary",
	})
	if err != nil {
		t.Fatal(err)
	}

	if created.RawContent != "raw completion summary" {
		t.Fatalf("RawContent = %q", created.RawContent)
	}
	if created.TrustLevel != "raw_model_output" {
		t.Fatalf("TrustLevel = %q", created.TrustLevel)
	}
	if created.Provenance != "task_completion" {
		t.Fatalf("Provenance = %q", created.Provenance)
	}
	if created.ReviewStatus != "pending_review" {
		t.Fatalf("ReviewStatus = %q", created.ReviewStatus)
	}
}

func TestMemoryStoreRanksByEmbeddingWhenQueryProvided(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore()
	squad := mustCreateMemoryTestSquad(t, ctx, store)
	agent := mustCreateMemoryTestAgent(t, ctx, store, squad.ID)

	olderCloser, err := store.CreateAgentMemory(ctx, &domain.AgentMemory{
		AgentID:   agent.ID,
		SquadID:   squad.ID,
		Content:   "closest semantic memory",
		Embedding: []float64{1, 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	newerFarther, err := store.CreateAgentMemory(ctx, &domain.AgentMemory{
		AgentID:   agent.ID,
		SquadID:   squad.ID,
		Content:   "newer but farther memory",
		Embedding: []float64{0, 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	recent, err := store.ListAgentMemory(ctx, agent.ID, squad.ID, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if recent[0].ID != newerFarther.ID {
		t.Fatalf("recent first ID = %q, want newer memory %q", recent[0].ID, newerFarther.ID)
	}

	semantic, err := store.ListAgentMemory(ctx, agent.ID, squad.ID, []float64{1, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if semantic[0].ID != olderCloser.ID {
		t.Fatalf("semantic first ID = %q, want closest memory %q", semantic[0].ID, olderCloser.ID)
	}
}

func mustCreateMemoryTestSquad(t *testing.T, ctx context.Context, store *MemoryStore) *domain.Squad {
	t.Helper()
	squad, err := store.CreateSquad(ctx, &domain.Squad{
		Name:    "memory-test",
		OwnerID: "owner-1",
		Status:  domain.SquadActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	return squad
}

func mustCreateMemoryTestAgent(t *testing.T, ctx context.Context, store *MemoryStore, squadID string) *domain.Agent {
	t.Helper()
	agent, err := store.CreateAgent(ctx, &domain.Agent{
		SquadID: squadID,
		Name:    "memory-agent",
		Status:  domain.AgentIdle,
	})
	if err != nil {
		t.Fatal(err)
	}
	return agent
}
