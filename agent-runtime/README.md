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
│   └── runtime.py         # core loop (TODO phase-3)
└── pyproject.toml
```
