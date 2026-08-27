# skquad Operator

The Kubernetes operator (Go, controller-runtime). Reconciles the `Squad` and
`Agent` custom resources: squad namespaces + base resources, agent Deployments
(scale-to-zero), secrets, and network policies.

- Design: [`docs/deployment-operator.md`](../docs/deployment-operator.md)
- Scale-to-zero: [`docs/adr/0003-scale-to-zero.md`](../docs/adr/0003-scale-to-zero.md)

## Layout
```
operator/
├── cmd/manager/main.go    # entrypoint
├── internal/api/v1/       # Squad, Agent CRDs
└── go.mod
```

## Current implementation

- `skquad.io/v1` `Squad` and `Agent` Kubernetes API types
- List types for both resources
- `runtime.Object` `DeepCopyObject` support for manager/client registration
- Scheme registration for `skquad.io/v1`
- controller-runtime manager entrypoint with health/readiness probes
- First `Squad` reconciler: creates/labels the configured squad namespace,
  creates the `skquad-agent` ServiceAccount, default-deny NetworkPolicy, DNS and
  selector-scoped API/gateway egress allow policy, namespace-local API Secret
  writer Role/RoleBinding, starter ResourceQuota, and records basic ready
  status. It adds a `skquad.io/squad-cleanup` finalizer and explicitly deletes
  managed cross-namespace resources plus the managed Namespace on CR deletion.
- First `Agent` reconciler: resolves the owning squad namespace, creates/updates
  the agent Deployment, scales up from `desiredActive`, waits for
  `status.idleSince + spec.idleTimeout` before scaling inactive agents to zero,
  uses the squad agent ServiceAccount, mounts agent credential/virtual-key
  Secrets when the Agent CR names them, passes runtime bootstrap env vars including
  provider ID, default model alias, control-plane/gateway URLs, and task/inbox
  polling settings plus runtime execution limits, enables the runtime task loop, configures
  `/healthz` and `/readyz` probes, writes basic ready status, and uses a
  `skquad.io/agent-cleanup` finalizer to delete the cross-namespace Deployment
  before the Agent CR is released
- Helm CRDs for `squads.skquad.io` and `agents.skquad.io`
- Helm operator templates for ServiceAccount, ClusterRole, ClusterRoleBinding,
  and Deployment

Next slices are checking Deployment available replicas before reporting ready
and configurable egress policy generation from registry grants.
