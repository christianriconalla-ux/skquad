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
│   └── runtime.py         # bootstrap, health/readiness, core loop/handler
├── tests/
│   └── test_runtime.py
└── pyproject.toml
```

## Current implementation

- Loads bootstrap config from the operator-provided `SKQUAD_*` environment.
- Reads mounted Kubernetes Secret directories for the agent credential and LLM
  gateway virtual key without returning raw secret values.
- Exposes `/healthz` and `/readyz` through FastAPI.
- Provides a small control-plane client for agent-authenticated task listing,
  resource discovery, claiming, completion/blocking, and idle/busy/error
  heartbeats.
- Provides `poll_once` for claim/heartbeat checks and `run_task_once` for the
  first handler-driven execution loop: claim a task, run an injected handler,
  complete to `in-review`/`done`, or block on handler failure.
- Provides a default `LiteLLMTaskHandler` that reads the mounted LLM gateway
  virtual key, calls the OpenAI-compatible gateway through LiteLLM, exposes
  registered plugin tool schemas, discovers granted active resources, and
  invokes loaded plugin tool calls.
- Starts the task loop in the runtime process when `SKQUAD_TASK_LOOP_ENABLED`
  is true (the operator sets it true for agent pods) while still serving
  `/healthz` and `/readyz`.
- Treats `/readyz` as an execution-readiness check: when the task loop is
  enabled, required control-plane/gateway config plus both mounted Secret values
  must be present.
- Provides the `skquad-agent-runtime` console script.

Inbox draining, dynamic plugin package loading, and memory store integration
are still upcoming slices.
