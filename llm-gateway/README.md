# skquad LLM Gateway

The central, model-agnostic LLM gateway (LiteLLM proxy, Python). All agent LLM
calls flow through it. It meters tokens, computes cost, enforces agent
permissions, and routes BYOM calls.

- Design: [`docs/llm-gateway.md`](../docs/llm-gateway.md)
- Decision: [`docs/adr/0002-llm-gateway.md`](../docs/adr/0002-llm-gateway.md)

## Layout
```
llm-gateway/
├── config.yaml            # LiteLLM proxy bootstrap config
└── pyproject.toml
```

## Current Implementation

- Runs the LiteLLM proxy with a `model_list` supplied through Helm values or
  this local bootstrap config.
- Enforces LiteLLM virtual-key authentication through `LITELLM_MASTER_KEY`.
- Uses the shared Postgres `DATABASE_URL` so LiteLLM can persist generated
  virtual keys and spend state.
- Includes `prisma` explicitly and generates LiteLLM's Prisma client during the
  image build because persistent virtual-key state requires Prisma binaries at
  proxy startup.
- Keeps provider credentials externalizable via environment variables or
  mounted Kubernetes Secrets referenced from `model_list` entries.

The Skquad control plane provisions agent virtual keys by calling LiteLLM
`/key/generate` with the active model aliases from the agent's granted
`llm_provider` resources. The raw virtual key is written only to the generated
agent Secret; Postgres stores the Secret ref, not the key value.

Metering callbacks into Skquad's own `metering_events` table and automatic key
updates when grants change are still follow-up slices.
