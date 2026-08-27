# Testing Strategy

skquad uses layered tests so the fast path can run on every pull request while
cluster admission and rollout checks stay explicit.

## CI-Runnable Gates

The standard GitHub Actions validation workflow runs:

- Go vet and unit tests for the control plane.
- Go vet and unit tests for the operator.
- Python runtime package install, unit tests, and bytecode compilation.
- LiteLLM gateway metadata, image, callback, Prisma, and startup smoke checks.
- Helm lint and chart rendering.
- Next.js web build.
- Root integration smoke tests.

Run the root integration smoke suite locally with:

```bash
python3 -m unittest discover -s tests/integration
```

The suite is intentionally small and fast. It runs representative control-plane
HTTP/runtime tests, Kubernetes outbox and CR-writer tests, operator reconciler
tests, agent-runtime tests, and a Helm render contract check. This catches
cross-component drift such as:

- generated credential and virtual-key refs no longer mapping into Agent CRs;
- Agent CR fields no longer matching operator env and mount expectations;
- task assignment no longer waking agent runtime Deployments;
- runtime task context and inbox contracts drifting from the API; and
- chart templates dropping required runtime, gateway, or web env wiring.

## Component Tests

Use component suites when editing one area:

```bash
(cd control-plane && go vet ./... && go test ./...)
(cd operator && go vet ./... && go test ./...)
(cd agent-runtime && python3 -m unittest discover -s tests)
(cd web && npm run lint && NEXT_TELEMETRY_DISABLED=1 npm run build)
helm lint charts/skquad
helm template skquad charts/skquad --namespace skquad-system --include-crds >/dev/null
```

## Cluster-Required Verification

These checks require a configured Kubernetes cluster and are not part of the
fast PR gate:

- Kubernetes server-side dry-run of chart renders.
- GitOps promotion into the lab cluster.
- ArgoCD sync/health verification.
- End-to-end runtime execution against a real LiteLLM provider and live agent
  pod.

For lab verification, export the lab kubeconfig first:

```bash
export KUBECONFIG=$HOME/projects/k3s-cluster/kubeconfig
helm template skquad charts/skquad --namespace skquad-system --include-crds \
  | kubectl apply --dry-run=server -f -
```

## Boundaries

The current integration suite does not replace the remaining roadmap work for
semantic memory or richer browser-level web tests. Those features should add
focused tests as they land.
