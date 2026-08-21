# skquad LLM Gateway

The central, model-agnostic LLM gateway (LiteLLM proxy, Python). All agent LLM
calls flow through it. It meters tokens, computes cost, enforces agent
permissions, and routes BYOM calls.

- Design: [`docs/llm-gateway.md`](../docs/llm-gateway.md)
- Decision: [`docs/adr/0002-llm-gateway.md`](../docs/adr/0002-llm-gateway.md)

## Layout
```
llm-gateway/
├── config.yaml            # LiteLLM proxy config (TODO phase-3)
└── pyproject.toml
```
