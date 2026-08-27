# skquad Helm Chart

Packages and deploys the skquad control plane + operator on vanilla Kubernetes
(no OLM). Squad/agent resources are created via the API (not by hand).

- Design: [`docs/deployment-operator.md`](../../docs/deployment-operator.md) §6

## Install
```
helm install skquad charts/skquad -n skquad-system --create-namespace
```

## Images

The chart defaults to GHCR images published by the repository workflows:

- `ghcr.io/rossbrigoli/skquad-api-server`
- `ghcr.io/rossbrigoli/skquad-operator`
- `ghcr.io/rossbrigoli/skquad-llm-gateway`
- `ghcr.io/rossbrigoli/skquad-agent-runtime`
- `ghcr.io/rossbrigoli/skquad-web`

Override `image.*.repository` and `image.*.tag` for local builds, private
registries, or a specific `sha-<short-sha>` image.

CRDs for `squads.skquad.io` and `agents.skquad.io` are installed from
`charts/skquad/crds` before the rest of the chart.

The current chart installs:

- CRDs for `Squad` and `Agent`
- Operator namespace, ServiceAccount, ClusterRole, ClusterRoleBinding, and
  Deployment. The ClusterRole includes CR finalizer update permissions and
  delete permissions for managed Namespaces, Deployments, ServiceAccounts,
  ResourceQuotas, and NetworkPolicies so operator finalizers can clean up
  cross-namespace resources.
- API server ServiceAccount, namespaced CR-writer RBAC, cluster-scoped
  agent-credential Secret writer RBAC, Deployment, and Service
- LLM gateway master-key Secret, ConfigMap, Deployment, and Service
- Web Deployment and Service
- Postgres Secret, Service, and StatefulSet for local/dev installs
- Optional Kubernetes Ingress routing `/api` to the API server and `/` to the
  web app
- Runtime URL defaults so generated Agent CRs point pods back at the chart's API
  server and LLM gateway services
- Agent runtime defaults for idle timeout, task poll interval, inbox poll
  interval, and inbox batch size.

For production installs, set `postgres.existingSecret` and
`apiServer.databaseUrlSecret.name` so database credentials and the API database
URL come from a pre-created Secret instead of chart values.

Override `apiServer.runtimeControlPlaneUrl` or
`apiServer.runtimeLlmGatewayUrl` when agent pods must call non-chart service
endpoints.

The chart starts the gateway as a LiteLLM proxy using
`llmGateway.config`. By default that config enables virtual-key auth through
`LITELLM_MASTER_KEY`, uses the chart/Postgres database with a separate
`litellm` schema, and has an empty `model_list` so the default render is valid
without real provider credentials.
Set `llmGateway.masterKeySecret.name` to use a pre-created Secret instead of
the development `llmGateway.masterKey` value. Set `llmGateway.databaseUrl` or
`llmGateway.databaseUrlSecret` to use an external LiteLLM database. Put provider
definitions in `llmGateway.config` and inject provider API keys with
`llmGateway.extraEnv` or `llmGateway.extraEnvFrom`; do not place real API keys
in the config map.
The gateway image includes LiteLLM proxy dependencies, generated Prisma client
artifacts, and a fetched Prisma query-engine binary for persistent virtual-key
storage.

`llmGateway.probes.startup` gives LiteLLM time to prepare database-backed proxy
state before liveness checks can restart the container. Tune it upward if a
large migration or cold image pull makes startup exceed the default window.
`llmGateway.migrationDir` sets `LITELLM_MIGRATION_DIR` to a writable path so
LiteLLM can create baseline migration state if the target schema is not empty.

Runtime polling defaults are configured through `agent.idleTimeout`,
`agent.taskPollIntervalSeconds`, `agent.inboxPollIntervalSeconds`, and
`agent.inboxBatchSize`. Runtime execution limits are configured through
`agent.taskTimeoutSeconds`, `agent.maxLLMSteps`, and
`agent.taskSummaryMaxChars`. The chart passes those values to the operator, and
the operator injects the corresponding `SKQUAD_*` env vars into generated agent
pods.

Public DNS/TLS values and external database hardening are still upcoming slices.
