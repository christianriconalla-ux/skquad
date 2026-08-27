# skquad CI/CD

> **Status:** Implemented validation CI; image publishing and deployment remain
> follow-up work.

The GitHub Actions workflow in `.github/workflows/ci.yml` is the main quality
gate for the monorepo. It runs on pushes to `main` and on pull requests.

## Validation Jobs

| Job | Checks |
|-----|--------|
| Control plane | `go vet ./...` and `go test ./...` in `control-plane/` |
| Operator | `go vet ./...` and `go test ./...` in `operator/` |
| Agent runtime | installs the Python package, runs `unittest`, and compiles runtime/test modules |
| LLM gateway | validates Python project metadata and required LiteLLM bootstrap config fields |
| Helm chart | `helm lint`, default chart render, and provider-configured gateway render |
| Web | deterministic `npm ci` and `next build` |

## Current Boundaries

- The workflow does not publish container images yet.
- The workflow does not deploy to Kubernetes.
- Kubernetes server-side dry-runs are still done locally against the lab
  cluster when a chart change needs API admission validation.
- The web job builds the current placeholder app shell; product UI coverage is
  tracked separately.

## Follow-Up Work

- Add container build and publish jobs for API server, operator, agent runtime,
  LLM gateway, and web images.
- Add integration/end-to-end tests once deployable images and test fixtures are
  available.
- Add release/deployment automation after the GitOps target is finalized.
