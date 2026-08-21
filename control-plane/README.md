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
├── internal/              # handlers, authn/authz, domain, CR writers
└── go.mod
```
