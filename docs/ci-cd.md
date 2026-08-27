# skquad CI/CD

> **Status:** Implemented validation CI and container image build/publish.
> Deployment automation remains follow-up work.

The GitHub Actions workflow in `.github/workflows/ci.yml` is the main quality
gate for the monorepo. It runs on pushes to `main` and on pull requests.

The image workflow in `.github/workflows/images.yml` builds deployable
containers for every charted component. Pull requests build the images without
publishing. Pushes to `main` and `v*.*.*` tags publish to GHCR. The workflow
also mirrors to Docker Hub when Docker Hub repository secrets are configured.

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

- The workflow does not deploy to Kubernetes.
- Kubernetes server-side dry-runs are still done locally against the lab
  cluster when a chart change needs API admission validation.
- The web job builds the current placeholder app shell; product UI coverage is
  tracked separately.

## Image Publishing

Published images use the GHCR namespace `rossbrigoli`:

| Component | Image |
|-----------|-------|
| API server | `ghcr.io/rossbrigoli/skquad-api-server` |
| Operator | `ghcr.io/rossbrigoli/skquad-operator` |
| Agent runtime | `ghcr.io/rossbrigoli/skquad-agent-runtime` |
| LLM gateway | `ghcr.io/rossbrigoli/skquad-llm-gateway` |
| Web | `ghcr.io/rossbrigoli/skquad-web` |

Tags:

- `sha-<short-sha>` on pushes.
- `latest` on the default branch.
- `<semver>` and `<major>.<minor>` on `v*.*.*` tags.

Docker Hub mirroring uses the namespace `brigss007` and expects GitHub Actions
secrets:

- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`

## Follow-Up Work

- Add integration/end-to-end tests once deployable images and test fixtures are
  available.
- Add release/deployment automation after the GitOps target is finalized.
