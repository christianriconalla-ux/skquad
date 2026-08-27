# Operator Install and Operations Runbook

This runbook covers the current Skquad Kubernetes install and day-two
operations. It describes implemented behavior only; production hardening items
that are still roadmap work are called out as boundaries.

## Scope

The Helm chart installs the control plane in one namespace, normally
`skquad-system`:

- API server
- web app
- LiteLLM gateway
- operator
- optional PostgreSQL
- `squads.skquad.io` and `agents.skquad.io` CRDs

The control plane API owns domain state in PostgreSQL. It writes Squad and
Agent custom-resource intents through a durable Kubernetes outbox. The operator
then reconciles those CRs into per-squad namespaces, base resources, and agent
Deployments.

## Prerequisites

- Kubernetes cluster access.
- Helm 3.
- Container registry access to the configured Skquad images.
- For non-development installs: pre-created Secrets for PostgreSQL, LiteLLM
  master key, OIDC configuration, and provider API keys.

For the lab cluster:

```bash
export KUBECONFIG=$HOME/projects/k3s-cluster/kubeconfig
```

## Install

Development install with chart-managed PostgreSQL and development auth:

```bash
helm upgrade --install skquad charts/skquad \
  --namespace skquad-system \
  --create-namespace
```

Verify control-plane workloads:

```bash
kubectl --namespace skquad-system get pods
kubectl --namespace skquad-system rollout status deployment/skquad-api-server
kubectl --namespace skquad-system rollout status deployment/skquad-operator
kubectl --namespace skquad-system rollout status deployment/skquad-llm-gateway
kubectl --namespace skquad-system rollout status deployment/skquad-web
```

Port-forward the API:

```bash
kubectl --namespace skquad-system port-forward service/skquad-api-server 8080:80
curl --fail http://127.0.0.1:8080/healthz
```

Default values are suitable for development only. They enable dev auth, create
a development PostgreSQL password, create a development LiteLLM master key, and
leave ingress disabled.

## Production-Oriented Values

Use external Secrets instead of chart-managed credentials:

```yaml
postgres:
  enabled: false
  existingSecret: skquad-postgres
  secretKeys:
    databaseUrl: database-url

apiServer:
  authMode: oidc
  databaseUrlSecret:
    name: skquad-postgres
    key: database-url
  oidc:
    issuer: https://issuer.example.com
    audience: skquad

llmGateway:
  masterKeySecret:
    name: skquad-litellm-master
    key: master-key
  databaseUrlSecret:
    name: skquad-litellm-db
    key: database-url
  extraEnv:
    - name: LOCAL_LLM_API_KEY
      valueFrom:
        secretKeyRef:
          name: local-llm-api-key
          key: api-key
```

Configure model aliases in `llmGateway.config` and grant providers/resources
through the Skquad API or web admin workflows. Do not put raw provider API keys
in ConfigMaps.

## Upgrade

Render and lint before upgrading:

```bash
helm lint charts/skquad
helm template skquad charts/skquad --namespace skquad-system --include-crds >/tmp/skquad-render.yaml
```

For lab GitOps, update the Skquad Application values in the k3s GitOps repo and
let ArgoCD apply the change. For a direct Helm environment:

```bash
helm upgrade skquad charts/skquad \
  --namespace skquad-system \
  -f values-prod.yaml
```

Watch rollout:

```bash
kubectl --namespace skquad-system rollout status deployment/skquad-api-server
kubectl --namespace skquad-system rollout status deployment/skquad-operator
kubectl --namespace skquad-system rollout status deployment/skquad-llm-gateway
kubectl --namespace skquad-system rollout status deployment/skquad-web
```

The chart currently installs CRDs from `charts/skquad/crds`. Review CRD changes
before upgrade; Helm does not remove CRDs on uninstall and CRD downgrade is not
automatic.

## Namespace Model

- `skquad-system` holds the control plane and all Squad/Agent CRs.
- Each Squad CR names one managed data-plane namespace.
- The operator creates each squad namespace, `skquad-agent` ServiceAccount,
  starter ResourceQuota, default-deny NetworkPolicy, DNS egress policy, and
  platform egress policy.
- Agent Deployments run in the squad namespace, not in `skquad-system`.

List managed CRs:

```bash
kubectl --namespace skquad-system get squads.skquad.io
kubectl --namespace skquad-system get agents.skquad.io
```

Inspect generated data-plane resources:

```bash
kubectl get ns --selector skquad.io/managed-by=skquad
kubectl --namespace squad-<id> get sa,deploy,netpol,resourcequota
```

## Generated Secrets

Agent identity create/rotate writes two Kubernetes Secrets into the squad
namespace:

- runtime credential Secret mounted at `/var/run/skquad/credentials/agent`;
- LiteLLM virtual-key Secret mounted at
  `/var/run/skquad/credentials/llm-gateway`.

The raw credential and virtual key are not stored in PostgreSQL. The control
plane stores the runtime credential verifier hash and the Kubernetes Secret
refs. Secret writes still happen synchronously during identity create/rotate
because the outbox intentionally does not persist raw token material.

Check Secret refs on an Agent CR:

```bash
kubectl --namespace skquad-system get agent <agent-cr-name> -o jsonpath='{.spec.credentialSecret}{"\n"}{.spec.virtualKeySecret}{"\n"}'
```

Check the mounted runtime pod contract without printing secret values:

```bash
kubectl --namespace squad-<id> get deploy <agent-cr-name> \
  -o jsonpath='{.spec.template.spec.volumes[*].secret.secretName}{"\n"}{.spec.template.spec.containers[0].volumeMounts[*].mountPath}{"\n"}'
```

## Ingress

Ingress is disabled by default:

```yaml
ingress:
  enabled: false
```

When enabled, the chart routes `/api` to the API server and `/` to the web app.
Do not expose a development-auth deployment publicly. For the lab environment,
create a Traefik IngressRoute or chart ingress for the internal `*.lab` host,
then map the public `*.cloud.rossbrigoli.com` host through Cloudflare Tunnel.

## Troubleshooting

### API Auth

Symptoms:

- every request is treated as the dev admin;
- OIDC requests return 401;
- user cannot access a squad they expect to see.

Checks:

```bash
kubectl --namespace skquad-system get deploy skquad-api-server \
  -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="SKQUAD_AUTH_MODE")].value}{"\n"}'
kubectl --namespace skquad-system logs deployment/skquad-api-server --tail=100
```

Current boundary: OIDC account identity still needs hardening to use
`issuer + subject` as the stable key. Do not treat mutable email alone as a
production-grade account binding yet.

### Kubernetes Outbox and CR Writer

Symptoms:

- API mutation succeeded but no Squad/Agent CR appears yet;
- Agent status changes lag behind task assignment;
- outbox failures accumulate.

Checks:

```bash
kubectl --namespace skquad-system logs deployment/skquad-api-server --tail=200
kubectl --namespace skquad-system get squads.skquad.io,agents.skquad.io
```

The API accepts domain mutations and queues Kubernetes intents. A temporary
Kubernetes API failure should not make the accepted domain mutation fail, but
Secret writes during identity create/rotate are still synchronous.

### Operator Reconciliation

Symptoms:

- squad namespace is missing;
- agent Deployment is missing or has stale env;
- finalizer is stuck.

Checks:

```bash
kubectl --namespace skquad-system logs deployment/skquad-operator --tail=200
kubectl --namespace skquad-system describe squad <squad-cr-name>
kubectl --namespace skquad-system describe agent <agent-cr-name>
kubectl --namespace squad-<id> describe deploy <agent-cr-name>
```

The operator uses explicit finalizers because Squad and Agent CRs live in
`skquad-system` while managed resources live in other namespaces. Do not remove
finalizers manually unless you have already cleaned up the managed resources.

### Agent Scale-to-Zero

Symptoms:

- agent never wakes for assigned work;
- agent stays running after becoming idle;
- unsupported messages keep an agent active.

Checks:

```bash
kubectl --namespace skquad-system get agent <agent-cr-name> \
  -o jsonpath='{.spec.desiredActive}{"\n"}{.status.idleSince}{"\n"}'
kubectl --namespace squad-<id> get deploy <agent-cr-name> \
  -o jsonpath='{.spec.replicas}{"\n"}'
```

Current boundary: unsupported `delegate`/`handoff` messages are retried and then
dead-lettered unless a specialized handler is installed. Automatic task
materialization for those message types is still a follow-up workflow slice.

### Runtime Readiness

Symptoms:

- agent pod is running but not ready;
- runtime cannot claim tasks;
- model calls fail with unauthorized gateway errors.

Checks:

```bash
kubectl --namespace squad-<id> logs deployment/<agent-cr-name> --tail=200
kubectl --namespace squad-<id> exec deploy/<agent-cr-name> -- printenv | grep '^SKQUAD_' | sort
kubectl --namespace squad-<id> get deploy <agent-cr-name> \
  -o jsonpath='{.spec.template.spec.containers[0].readinessProbe.httpGet.path}{"\n"}'
```

Do not print Secret contents in shared logs. Verify only the Secret names and
mount paths unless actively debugging on a private terminal.

## Security and RBAC Notes

- The operator needs cluster-scope access for namespaces and cross-namespace
  managed resources.
- The API server currently needs Secret write/delete access for generated
  agent credential and virtual-key Secrets. Narrower namespace-scoped authority
  remains follow-up hardening.
- Agent pods receive only mounted runtime credentials and LiteLLM virtual keys;
  they do not receive provider API keys directly.
- NetworkPolicy currently permits DNS and platform egress. Registry-derived
  external egress policies are still future work.
- Do not enable public ingress while `apiServer.authMode=dev`.

## Uninstall

For a direct Helm install:

```bash
helm uninstall skquad --namespace skquad-system
```

Helm does not remove CRDs by default. Before deleting CRDs, confirm all managed
Squad and Agent CRs are gone and their finalizers have cleaned up data-plane
resources:

```bash
kubectl --namespace skquad-system get squads.skquad.io,agents.skquad.io
kubectl get ns --selector skquad.io/managed-by=skquad
```

If you used chart-managed PostgreSQL, uninstalling the release can remove the
database workload. Preserve or back up persistent volumes before destructive
cleanup.
