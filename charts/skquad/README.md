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
- API server ServiceAccount, namespaced CR-writer RBAC, Deployment, and Service
- LLM gateway ConfigMap, Deployment, and Service
- Web Deployment and Service
- Postgres Secret, Service, and StatefulSet for local/dev installs

Public ingress and production-grade secret management are still upcoming
slices.
