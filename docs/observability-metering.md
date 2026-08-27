# skquad — Observability & Metering Design

> **Status:** Draft v1
>
> skquad has two related but distinct features:
> - **Observability** — an **optional** feature that emits **Prometheus
>   metrics** (enabled per deployment).
> - **Metering** — **token usage** per agent and per squad, with **monetary
>   cost** when a provider has per-token pricing. Current metering is captured
>   through the **LLM gateway** callback path where configured.
>
> Runtime metrics and gateway metering callbacks have an initial implementation;
> full production observability remains a design target. See
> [`implementation-status.md`](implementation-status.md).

---

## 1. Observability (Prometheus, optional)

- An **observability feature** can be **enabled** (via Helm values) to emit
  **Prometheus metrics**.
- When enabled, a **Prometheus** instance (or the cluster's existing one)
  scrapes metrics from the control plane, operator, and (when up) agent pods.
- When disabled, no metrics are emitted (minimal footprint).

### 1.1 What is exposed

| Source | Metrics |
|--------|---------|
| **API server** | request rate, latency, error rate, active sessions. |
| **LLM gateway** | requests, tokens (in/out), cost, latency, errors, per provider/model. |
| **Operator** | reconcile rate, scale-up/down latency, time-at-zero, wake count, errors. |
| **Agent pods** | task duration, LLM calls, tool calls, memory size, idle/busy time. |
| **Postgres** | connections, query latency (via exporter). |

### 1.2 Key metrics (examples)

```
skquad_api_requests_total{method, route, code}
skquad_api_request_duration_seconds{route}
skquad_llm_requests_total{provider, model, agent, squad}
skquad_llm_tokens_total{provider, model, agent, squad, direction}  # direction=in|out
skquad_llm_cost_total{provider, model, agent, squad}
skquad_llm_request_duration_seconds{provider, model}
skquad_operator_reconcile_total{kind, result}
skquad_operator_scale_up_duration_seconds{agent}
skquad_agent_tasks_total{agent, squad, status}
skquad_agent_task_duration_seconds{agent, squad}
skquad_agent_task_errors_total{agent, squad}
skquad_agent_task_timeouts_total{agent, squad}
skquad_agent_inbox_messages_total{agent, squad, result}
skquad_agent_state{agent, squad}  # 0=idle, 1=busy
```

The current runtime exposes dependency-free Prometheus text at `/metrics` and a
JSON snapshot at `/status`. The implemented counters cover readiness,
task-claim/completion/block totals, execution errors, execution timeouts,
cumulative task duration, inbox fetched/processed/failed totals, loop errors,
and whether a task is active. Labels are limited to agent and squad IDs; task
payloads, message payloads, summaries, and credentials are intentionally not
exported.

### 1.3 Dashboards & alerts

- **Dashboards** (Grafana, optional): platform health, LLM spend, squad
  activity, operator health.
- **Alerts:** gateway error rate, operator reconcile failures, Postgres health,
  spend anomalies.

---

## 2. Metering (token usage + cost)

Metering is **always on** (it is a core feature, not optional) and is captured
centrally at the **LLM gateway** (see [llm-gateway.md](llm-gateway.md)).

### 2.1 What is metered

For **every LLM call** an agent makes (via the gateway), the gateway records:

- **agent id**, **squad id**
- **task id** when the call happens while executing a task
- **model**, **provider**
- **input tokens**, **output tokens**
- **timestamp**
- **cost** (if the provider has per-token pricing)

Current implementation: the runtime attaches `skquad_agent_id`,
`skquad_squad_id`, and `skquad_task_id` metadata to LiteLLM calls. The LiteLLM
proxy loads the Skquad custom callback and posts successful calls to
`POST /api/v1/gateway/metering` with the gateway callback token. Failed gateway
calls produce a best-effort system audit entry instead of a usage row.

### 2.2 Cost calculation

```
cost = input_tokens × price_per_input_token
     + output_tokens × price_per_output_token
```

- Prices come from the **provider's registry entry** (`pricing`).
- If a provider has **no pricing configured**, cost is **not shown** (tokens
  only).
- Cost is computed **per call** and **aggregated** per agent / per squad / per
  time period.

### 2.3 Storage

- Metering records are written to the **metering table** in Postgres (see
  [data-model.md](data-model.md)).
- The table is **partitioned by time** (for retention + query performance).

```
metering(
  id,
  agent_id, squad_id, task_id,
  model, provider,
  input_tokens, output_tokens,
  cost,            # nullable (null if no pricing)
  currency,
  timestamp
)  -- partitioned by timestamp
```

### 2.4 Aggregation & display

- The **web app** shows metering:
  - **Per agent** — tokens + cost over time.
  - **Per squad** — aggregate tokens + cost.
  - **Per provider/model** — breakdown.
- Aggregations are computed from the metering table (with time-range filters).
- The **platform admin** can view metering across all squads.

Callback delivery is intentionally asynchronous and best-effort from the
gateway's perspective so LLM responses are not blocked by Skquad metering
ingestion outages. The control plane treats successful usage inserts as the
durable accounting event and records a follow-up audit row on a best-effort
basis, matching current mutation-audit semantics.

---

## 3. Observability vs Metering

| | Observability | Metering |
|---|---------------|----------|
| **Purpose** | Platform health / debugging | Usage / cost accounting |
| **Optional?** | Yes (toggle) | No (always on) |
| **Source** | Prometheus (scraped) | LLM gateway (written to Postgres) |
| **Granularity** | Time-series metrics | Per LLM call |
| **Audience** | Operators / admins | Owners / admins (cost) |

---

## 4. Relationship to Other Components

- **LLM gateway** — the single source of metering data (tokens + cost).
- **Postgres** — stores metering (partitioned) + audit.
- **Prometheus** — (optional) scrapes metrics when observability is enabled.
- **Web app** — displays metering (per agent/squad) + (optional) dashboards.
- **Resource registry** — provides provider pricing (for cost).
- **Operator** — emits scale-up/down + reconcile metrics.

---

## 5. Retention & Performance

- **Metering** — partitioned by time; retention policy configurable (e.g. keep
  raw for N months, then aggregate + drop).
- **Audit log** — retained longer (compliance).
- **Prometheus** — standard retention (configurable).
- **Query performance** — aggregate views are pre-computed or indexed; time-range
  filters use the partition key.

---

## 6. Open Points

- **Budgets** — hard spend caps per agent/squad with automatic cutoff (later;
  the gateway supports it).
- **Cost allocation** — per-user or per-project cost breakdown (later).
- **Export** — export metering to a billing system / CSV (later).
- **Streaming token counts** — confirm how streaming responses report token
  counts (via the gateway).
