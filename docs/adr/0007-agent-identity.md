# ADR-0007: Agent Identity — Owner-Created, Platform-Facilitated

- **Status:** Accepted
- **Date:** 2026-08-22
- **Deciders:** Ross, Sherlock

## Context

Each agent has its **own identity and credentials** (FR-3). Ross specified that
**the human user who owns the agent creates the agent's identity** and
configures the agent to use its credential. This is a "bring your own agent
identity" model (analogous to BYOM).

Tension (D4): the "minutes" onboarding flow must stay fast, but creating an
agent identity is a manual step. We need to reconcile owner-ownership with a
fast path.

## Decision

- An **`AgentIdentity`** is a first-class entity, **created by the squad
  owner** (not auto-provisioned by the platform).
- The platform **facilitates** creation with a **one-click "create agent
  identity"** action: it generates the identity + a credential (stored as a K8s
  secret in the squad namespace) and attaches it to the agent. The **owner
  owns** it (can rotate, replace, or delete).
- The agent's **permissions** (which registry resources it may use) are set by
  the **squad owner**, independent of the identity.
- The credential is scoped to the agent and used by the agent runtime to call
  the LLM gateway and permitted resource connectors.

This keeps the owner in control (security requirement) while the one-click
facilitation keeps the onboarding path fast (D4 resolved).

## Consequences

- **(+)** Owner control over agent identity + credentials → strong security
  posture, clear accountability.
- **(+)** One-click facilitation → the "minutes" flow stays fast.
- **(+)** Identity is a distinct entity → credentials/permissions can be
  managed independently of runtime config (rotation without re-creating the
  agent).
- **(+)** Credential stored as a K8s secret in the squad namespace → isolated
  per squad.
- **(−)** Slightly more onboarding surface than auto-provisioning.
- **(−)** Owner must understand identity/credential rotation.
- **Mitigation:** clear UX + docs for the one-click flow and rotation; audit
  all identity/credential changes.

## Alternatives Considered

- **Platform auto-provisions agent identity** — fastest, but the owner does not
  control the identity/credential, conflicting with the security requirement.
  **Rejected** per Ross's requirement.
- **K8s ServiceAccount per agent** — simple, but ties agent identity to the K8s
  layer and does not model owner ownership/rotation well. **Rejected** as the
  primary model (may still be used internally for pod auth).
- **SPIFFE/mTLS** — strong workload identity, but adds significant
  infrastructure. **Rejected** for v1; revisit for hardened deployments.
