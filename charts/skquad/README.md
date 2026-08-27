# skquad Helm Chart

Packages and deploys the skquad control plane + operator on vanilla Kubernetes
(no OLM). Squad/agent resources are created via the API (not by hand).

- Design: [`docs/deployment-operator.md`](../../docs/deployment-operator.md) §6

## Install
```
helm install skquad charts/skquad -n skquad-system --create-namespace
```

CRDs for `squads.skquad.io` and `agents.skquad.io` are installed from
`charts/skquad/crds` before the rest of the chart.

The current chart installs:

- CRDs for `Squad` and `Agent`
- Operator namespace, ServiceAccount, ClusterRole, ClusterRoleBinding, and
  Deployment
- API server ServiceAccount, namespaced CR-writer RBAC, cluster-scoped
  agent-credential Secret writer RBAC, Deployment, and Service
- LLM gateway ConfigMap, Deployment, and Service
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

Runtime polling defaults are configured through `agent.idleTimeout`,
`agent.taskPollIntervalSeconds`, `agent.inboxPollIntervalSeconds`, and
`agent.inboxBatchSize`. The chart passes those values to the operator, and the
operator injects the corresponding `SKQUAD_*` env vars into generated agent
pods.

Public DNS/TLS values and external database hardening are still upcoming slices.
