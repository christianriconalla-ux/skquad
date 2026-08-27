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
├── skquad_litellm_callbacks.py
└── pyproject.toml
```

## Current Implementation

- Runs the LiteLLM proxy with a `model_list` supplied through Helm values or
  this local bootstrap config.
- Enforces LiteLLM virtual-key authentication through `LITELLM_MASTER_KEY`.
- Uses `DATABASE_URL` so LiteLLM can persist generated virtual keys and spend
  state. The Helm chart targets a separate `litellm` Postgres schema when it
  uses the bundled database.
- Includes `prisma` explicitly, generates LiteLLM's Prisma client during the
  image build, and fetches the Prisma query engine into the non-root runtime
  user's cache because persistent virtual-key state requires Prisma binaries at
  proxy startup.
- Uses `LITELLM_MIGRATION_DIR` for writable LiteLLM migration/baseline state at
  runtime.
- Keeps provider credentials externalizable via environment variables or
  mounted Kubernetes Secrets referenced from `model_list` entries.

The Skquad control plane provisions agent virtual keys by calling LiteLLM
`/key/generate` with the active model aliases from the agent's granted
`llm_provider` resources. The raw virtual key is written only to the generated
agent Secret; Postgres stores the Secret ref, not the key value.

The proxy loads `skquad_litellm_callbacks.proxy_handler_instance` as a LiteLLM
custom callback. Successful calls post token/cost usage to the control plane's
internal `/api/v1/gateway/metering` endpoint; failed calls post an audit-only
event. The callback includes agent, squad, and task metadata supplied by the
runtime and authenticates with `SKQUAD_GATEWAY_CALLBACK_TOKEN`.

Automatic key updates when grants change are still a follow-up slice.
