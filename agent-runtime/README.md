# skquad Agent Runtime

The thin custom agent harness (Python, LiteLLM + plugin interface). Runs inside
each agent pod. Owns the agent's lifecycle, task-scoped context, the core work
loop, and the plugin interface. Model-agnostic and extensible.

- Design: [`docs/agent-runtime.md`](../docs/agent-runtime.md)
- Decision: [`docs/adr/0001-agent-runtime.md`](../docs/adr/0001-agent-runtime.md)

## Layout
```
agent-runtime/
├── skquad_runtime/
│   ├── __init__.py
│   └── runtime.py         # bootstrap, health/readiness, core loop shell
├── tests/
│   └── test_runtime.py
└── pyproject.toml
```

## Current implementation

- Loads bootstrap config from the operator-provided `SKQUAD_*` environment.
- Reads mounted Kubernetes Secret directories for the agent credential and
  optional LLM gateway virtual key without returning raw secret values.
- Exposes `/healthz` and `/readyz` through FastAPI.
- Provides a small control-plane client for agent-authenticated task listing,
  claiming, completion/blocking, and idle/busy/error heartbeats.
- Provides `poll_once`, the first runtime work-loop primitive: claim the next
  assigned task and report idle/busy without faking task execution.
- Provides the `skquad-agent-runtime` console script.

The full task execution loop, plugin loader, LiteLLM calls, inbox draining, and
memory store are still upcoming slices.
