# ADR-0008: Language & Runtime Choices

- **Status:** Accepted
- **Date:** 2026-08-22
- **Deciders:** Ross, Sherlock

## Context

skquad has several components with different needs:

- **Control-plane API server** + **operator** — K8s-native, need strong typing,
  concurrency, and the controller-runtime ecosystem.
- **Agent runtime** — needs LiteLLM (Python-first) and a rich plugin/ML
  ecosystem.
- **LLM gateway** — LiteLLM proxy (Python).
- **Web app** — needs a fast-to-build SPA.

We want to minimise the number of languages while matching each component to
its best-fit ecosystem.

## Decision

| Component | Language / Runtime | Rationale |
|-----------|--------------------|-----------|
| API server | **Go** | K8s-native, shares language + libraries with the operator, strong typing, easy concurrency. |
| Operator | **Go** (controller-runtime / kubebuilder) | Standard for K8s operators. |
| Agent runtime | **Python** | LiteLLM is Python-first; rich plugin/ML ecosystem. |
| LLM gateway | **Python** (LiteLLM proxy) | Model-agnostic, virtual keys, budgets, usage built in. |
| Web app | **TypeScript** (React / Next.js) | Fast to build, large ecosystem. |

So the platform uses **two primary languages**: **Go** (control plane +
operator) and **Python** (agent runtime + gateway), plus **TypeScript** for the
web app.

## Consequences

- **(+)** Each component uses its best-fit ecosystem.
- **(+)** Go for the control plane + operator → one language, shared K8s
  libraries, coherent.
- **(+)** Python for the agent runtime + gateway → LiteLLM is natural, plugins
  are easy.
- **(+)** TypeScript for the web app → fast to build.
- **(−)** Three languages across the system → more tooling/CI surface.
- **Mitigation:** clear monorepo layout (one dir per component), shared CI
  templates, and language-specific lint/test configs.

## Alternatives Considered

- **All-Go** (agent runtime in Go) — one language, but LiteLLM and the ML/plugin
  ecosystem are Python-first; re-implementing model-agnostic calls in Go is
  costly. **Rejected.**
- **All-Python** (API server + operator in Python) — one language, but the K8s
  operator ecosystem (controller-runtime) is Go-first; Python K8s clients are
  less mature for operators. **Rejected.**
- **Node/TypeScript for the API server** — viable, but Go is a better fit for a
  K8s-native control plane that shares code with the operator. **Rejected.**
