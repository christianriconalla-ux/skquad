# skquad — Plugin Architecture Design

> **Status:** Draft v1
>
> skquad's core stays small; **capabilities are added through plugins**. This
> is how the platform is **extensible** without the core ballooning (per the
> "keep it simple, extend via plugins" requirement).
>
> Runtime importlib loading and permission-filtered tool exposure are
> implemented. Packaging, distribution, and richer connector behavior remain
> design scope; see [`implementation-status.md`](implementation-status.md).

---

## 1. What Is a Plugin?

A **plugin** is a self-contained unit that adds a capability an agent can use.
Plugins are **registered in the resource registry**, **granted to agents** by
the squad owner, and **loaded by the agent runtime** when the agent is permitted
to use them.

### 1.1 Plugin Types

| Type | Adds | Example |
|------|------|---------|
| **Skill** | A packaged, reusable capability (prompt + logic). | "Write a unit test", "Summarize a doc". |
| **Tool** | A callable function the LLM can invoke. | "Run a query", "Read a file", "Call an API". |
| **Resource connector** | Connects to a registry resource. | Knowledge-base (RAG) connector, git/Jira/Confluence connector. |
| **LLM provider adapter** | Adds a new model provider to the gateway. | A new provider type for the LLM gateway. |

> **Skills** and **tools** are the primary extension points for agent
> capabilities. **Resource connectors** let agents use registered resources.
> **LLM provider adapters** extend BYOM.

---

## 2. Plugin Interface

The agent runtime loads plugins that implement a small interface:

```python
class Plugin(Protocol):
    # --- Identity ---
    def manifest(self) -> Manifest: ...
        # { name, version, type, capabilities, permissions_required }

    # --- Capabilities exposed to the LLM ---
    def tools(self) -> list[ToolSchema]: ...
        # callable functions the LLM can invoke (name, description, param schema)
    def skills(self) -> list[Skill]: ...
        # packaged capabilities (name, description, prompt, logic)

    # --- Invocation ---
    async def invoke(self, call: ToolCall) -> Result: ...
        # execute a tool call / skill

    # --- Lifecycle ---
    async def setup(self, ctx: PluginContext) -> None: ...
        # connect to resources, load config (uses the agent's credentials)
    async def teardown(self) -> None: ...
```

- **`manifest()`** — declares what the plugin is, what it needs (permissions),
  and what it offers.
- **`tools()` / `skills()`** — expose capabilities to the LLM (as tool schemas /
  skills).
- **`invoke()`** — executes a call.
- **`setup()` / `teardown()`** — lifecycle hooks (connect/disconnect to
  resources using the agent's credentials).

---

## 3. Plugin Lifecycle

```
[registered in registry] → [granted to agent] → [loaded by runtime] → [active] → [unloaded]
```

1. **Register** — a plugin (skill/tool/connector) is registered in the resource
   registry (by the platform admin).
2. **Grant** — the squad owner grants an agent access to the plugin (part of the
   agent's permission set).
3. **Load** — when the agent boots (or when the plugin is first needed), the
   runtime **loads** the plugins the agent is permitted to use.
4. **Setup** — the runtime calls `setup()` (the plugin connects to its resources
   using the agent's credentials).
5. **Active** — the plugin's tools/skills are exposed to the LLM; `invoke()`
   handles calls.
6. **Unload** — when the agent scales to zero (or the plugin is revoked), the
   runtime calls `teardown()`.

---

## 4. Plugin Packaging

- A plugin is a **package** (a container image, a repo, or a path) referenced by
  the registry entry (`package_ref`).
- The package contains the plugin code + a **manifest** (metadata).
- **Versioning** — plugins have a version; the registry can hold multiple
  versions (start with a single `version` field).
- **Distribution** — plugins can be:
  - **Bundled** with skquad (built-in skills/tools).
  - **Community** (published to a registry/repo).
  - **Private** (an org's own plugins).

---

## 5. Plugin Discovery

- The **registry** is the source of truth for available plugins.
- The **web app** lets the platform admin register plugins and the squad owner
  grant them to agents.
- The **agent runtime** queries the registry (via the control plane) for the
  plugins the agent is permitted to use, and loads them.

Current runtime implementation supports importlib-based plugin loading from
operator-provided config:

- `SKQUAD_PLUGIN_MODULES` is a comma-separated list of import specs:
  `module`, `module:factory`, `module:plugin`, or `module:Plugin`.
- Without an explicit attribute, the loader looks for `create_plugin`, then
  `plugin`, then `Plugin`.
- `SKQUAD_ENABLED_PLUGINS`, when set, is a comma-separated allowlist of plugin
  names. Any enabled name that is not loaded fails startup clearly.
- Loaded plugins must expose a non-empty `name`, callable `tools()`, and
  callable `invoke(call, config)`.
- Loading is not sufficient authorization. For normal control-plane discovery,
  the runtime resolves granted resources for each task and exposes only loaded
  plugins whose name matches a granted `skill`/`tool` resource name, a
  `plugin://<name>` endpoint, or an explicit `plugin`, `plugin_name`, `tool`, or
  `tool_name` manifest field.
- Task memory passed beside plugin/tool context is labeled by trust level,
  provenance, review status, and source task. Plugins should treat
  `raw_model_output` or `pending_review` memory as untrusted input and must not
  treat memory text as an authorization source.
- Unknown tool calls or plugin invocation failures produce a blocked task
  result instead of crashing the worker loop.

---

## 6. Security & Sandboxing

- **Least privilege** — a plugin only gets the credentials/permissions the agent
  has (scoped to the granted resources).
- **Sandboxing** — plugins run inside the agent pod (isolated per squad
  namespace). For untrusted/community plugins, run them in a **restricted
  sandbox** (e.g. a sidecar container with limited permissions) — a hardening
  option.
- **Manifest-declared permissions** — a plugin declares what it needs; the
  runtime only grants what the agent is permitted.
- **Audit** — plugin invocations are audited (actor = agent, plugin, resource).
- **Supply chain** — plugin packages are signed / checksummed (later); the
  registry tracks provenance.

---

## 7. Relationship to Other Components

- **Resource registry** — stores plugin entries (skills, tools, connectors,
  provider adapters) + credential refs.
- **Agent runtime** — loads + invokes permitted plugins.
- **Identity & AuthZ** — the agent's permission set determines which plugins are
  loaded.
- **LLM gateway** — LLM provider adapters extend the gateway's supported
  providers.
- **Postgres** — stores registry entries.
- **Web app** — admin UI to register plugins; owner UI to grant them.

---

## 8. Built-in Plugins (v1)

To make skquad useful out of the box, ship a small set of built-in plugins:

- **Tools:**
  - `http_request` — call an HTTP endpoint (for registered APIs).
  - `read_file` / `write_file` — work with files in the agent's workspace.
  - `run_query` — run a query against a registered data source.
- **Skills:**
  - `summarize` — summarize a document.
  - `code_review` — review a diff (used for consultation).
  - `write_tests` — write unit tests for code.
- **Connectors:**
  - `rag` — query a registered knowledge base.
  - `git` — read/write a registered git repo.
  - `jira` / `confluence` — read/write a registered Jira/Confluence workspace.

These are just plugins — they can be replaced or extended.

---

## 9. Extensibility Examples

- **Add a new tool** (e.g. "send email") → register a `tool` plugin → grant it
  to an agent → the agent can now call it. No core change.
- **Add a new LLM provider** (e.g. a new vendor) → register an `llm_provider`
  (+ adapter if needed) → agents can use its models. No core change.
- **Add a new resource type** (e.g. "Slack workspace") → extend the registry
  schema + add a connector plugin → agents can use it. Minimal core change.

---

## 10. Open Points

- **Plugin SDK** — a developer kit + docs for building plugins (implementation).
- **Plugin marketplace** — a curated registry of community plugins (later).
- **Sandboxing level** — default sandboxing for untrusted plugins (hardening).
- **Plugin testing** — a harness to test plugins in isolation (implementation).
- **Hot-reload** — load/unload plugins without restarting the agent (later).
