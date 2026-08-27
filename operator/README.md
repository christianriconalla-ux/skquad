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
  platform egress allow policies, starter ResourceQuota, and records basic ready
  status
- First `Agent` reconciler: resolves the owning squad namespace, creates/updates
  the agent Deployment, sets scale-to-zero replicas from `desiredActive`, uses
  the squad agent ServiceAccount, mounts optional agent credential/virtual-key
  Secrets, passes runtime bootstrap env vars, configures `/healthz` and
  `/readyz` probes, and writes basic ready status
- Helm CRDs for `squads.skquad.io` and `agents.skquad.io`
- Helm operator templates for ServiceAccount, ClusterRole, ClusterRoleBinding,
  and Deployment

Next slices are control-plane/runtime chart templates, configurable egress
policy generation from registry grants, and the agent runtime bootstrap
contract.
