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
