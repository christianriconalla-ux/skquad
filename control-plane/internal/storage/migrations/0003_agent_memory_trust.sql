-- Harden agent memory with explicit trust/provenance labels and embedding
-- metadata. The existing vector column remains nullable so embeddings can be
-- disabled by configuration until an embedding provider is wired.
ALTER TABLE agent_memory
    ADD COLUMN IF NOT EXISTS raw_content text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trust_level text NOT NULL DEFAULT 'raw_model_output',
    ADD COLUMN IF NOT EXISTS provenance text NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS review_status text NOT NULL DEFAULT 'pending_review',
    ADD COLUMN IF NOT EXISTS embedding_model text NOT NULL DEFAULT '';

ALTER TABLE agent_memory
    DROP CONSTRAINT IF EXISTS agent_memory_trust_level_check,
    ADD CONSTRAINT agent_memory_trust_level_check
        CHECK (trust_level IN ('raw_model_output', 'distilled', 'verified'));

ALTER TABLE agent_memory
    DROP CONSTRAINT IF EXISTS agent_memory_review_status_check,
    ADD CONSTRAINT agent_memory_review_status_check
        CHECK (review_status IN ('pending_review', 'approved', 'rejected'));
