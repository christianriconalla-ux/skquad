# skquad Control Plane (API Server)

The REST API server (Go). Single entry point: OIDC authN, user RBAC, access
grants, domain CRUD, and creation of the `Squad`/`Agent` CRs the operator
reconciles.

- Design: [`docs/api-design.md`](../docs/api-design.md)
- Identity & RBAC: [`docs/identity-security.md`](../docs/identity-security.md)

## Layout
```
control-plane/
├── cmd/api/main.go        # entrypoint
├── internal/httpapi/      # REST handlers and request auth boundary
├── internal/storage/      # storage interfaces plus dev in-memory store
├── internal/domain/       # domain entities
├── internal/config/       # env config loader
└── go.mod
```

## Current implementation

The API server is runnable in development mode:

```bash
SKQUAD_ADDR=127.0.0.1:8080 go run ./cmd/api
```

By default it uses a process-local in-memory store. Set
`SKQUAD_DATABASE_URL` to use Postgres; embedded idempotent migrations are
applied at startup.

Authentication defaults to `SKQUAD_AUTH_MODE=dev`. Set `SKQUAD_AUTH_MODE=oidc`
with `SKQUAD_OIDC_ISSUER` and `SKQUAD_OIDC_AUDIENCE` to validate OIDC bearer
JWTs and provision users from token claims.

Kubernetes custom-resource writes are disabled by default. Set
`SKQUAD_K8S_ENABLED=true` to mirror squad/agent mutations into the Kubernetes
API for the operator. Optional settings:

- `SKQUAD_K8S_API_BASE` (default `https://kubernetes.default.svc`)
- `SKQUAD_K8S_NAMESPACE` (default `skquad-system`)
- `SKQUAD_K8S_TOKEN_FILE` (default service-account token path)
- `SKQUAD_K8S_GROUP_VERSION` (default `skquad.io/v1`)
- `SKQUAD_K8S_INSECURE` (default `false`, development only)
- `SKQUAD_AGENT_IMAGE` (default `skquad/agent-runtime:0.1.0`)
- `SKQUAD_CONTROL_PLANE_URL` (optional runtime callback URL written to Agent CRs)
- `SKQUAD_LLM_GATEWAY_URL` (optional gateway URL written to Agent CRs)

Implemented slice:

- `GET /healthz`
- `GET /api/v1/auth/me` with dev-mode principal provisioning
- Squad CRUD under `/api/v1/squads`
- Agent CRUD for squads
- Agent identity create/rotate endpoints with generated credential Secret
  material, stored verifier hashes, and public secret/key references
- Agent registry permission list/set endpoints
- Agent-facing `/api/v1/agents/me` endpoints for task listing, task claiming,
  granted active resource discovery, task start/complete/block, and status
  heartbeats
- Board retrieval and task create/update/move/delete
- Access grant create/list/delete
- LLM provider registry create/list/get/update/deprecate
- Generic registry resource create/list/get/update/deprecate for skills, tools,
  APIs, knowledge bases, and project workspaces
- Metering reads for squad, agent, and platform-admin summary views
- Audit log reads for squad owners and platform admins
- Best-effort audit recording for control-plane mutations
- Optional Kubernetes `Squad`/`Agent` CR writer for squad/agent mutations,
  including the generated squad namespace in `Squad.spec.namespace`
- Consistent JSON error envelopes
- Process-local in-memory store for development and handler tests
- Postgres-backed user/squad/agent/identity/permission/board/task/access-grant/
  registry, metering, and audit storage
- OIDC bearer JWT validation with first-login user provisioning
- Owner/admin/granted-user read authorization for squad resources
- Platform-admin authorization for registry writes
- Agent credential auth for runtime endpoints using one-way credential verifier
  hashes, with credential-ref matching retained only for old rows without hashes
- Assigned `todo`/`in-progress` task state mirrored into the `Agent` CR so the
  operator can scale an agent up or keep it warm while work remains

Next slices are the full runtime task execution loop and idle-timeout-aware
operator scale-down.
