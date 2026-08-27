"""Fast cross-component integration smoke tests for skquad.

These tests deliberately stay CI-friendly: they use unit-test fakes and Helm
renders instead of requiring a live Kubernetes cluster. Cluster admission and
full rollout checks remain separate operational verification.
"""

from __future__ import annotations

import os
import subprocess
import sys
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]


class IntegrationSmokeTest(unittest.TestCase):
    maxDiff = 2000

    def test_control_plane_runtime_operator_contracts(self) -> None:
        """Exercise the core API -> outbox -> operator -> runtime contracts."""

        self.run_command(
            [
                "go",
                "test",
                "./internal/httpapi",
                "-run",
                (
                    "TestAgentIdentityCreateAndRotate|"
                    "TestAgentRuntimeTaskClaimAndStatusFlow|"
                    "TestAgentRuntimeTaskContextIncludesScopedMemory|"
                    "TestAssignedTaskMirrorsAgentBusyAndCompletionMirrorsIdle|"
                    "TestAgentMessagingInboxFlow"
                ),
            ],
            cwd=REPO_ROOT / "control-plane",
        )
        self.run_command(
            [
                "go",
                "test",
                "./internal/kube",
                "-run",
                (
                    "TestProcessOutboxOnceAppliesQueuedSquadAndAgentEvents|"
                    "TestUpsertAgentMapsGeneratedSecretRefs|"
                    "TestWriteAgentCredentialAppliesOpaqueSecret"
                ),
            ],
            cwd=REPO_ROOT / "control-plane",
        )
        self.run_command(
            [
                "go",
                "test",
                "./internal/controller",
                "-run",
                (
                    "TestSquadReconcilerCreatesNamespace|"
                    "TestAgentReconcilerCreatesDeployment|"
                    "TestAgentReconcilerFinalizerDeletesDeployment"
                ),
            ],
            cwd=REPO_ROOT / "operator",
        )
        self.run_command(
            [sys.executable, "-m", "unittest", "discover", "-s", "tests"],
            cwd=REPO_ROOT / "agent-runtime",
            env={"PYTHONPATH": str(REPO_ROOT / "agent-runtime")},
        )

    def test_helm_render_preserves_runtime_and_gateway_contracts(self) -> None:
        rendered = self.run_command(
            [
                "helm",
                "template",
                "skquad",
                "charts/skquad",
                "--namespace",
                "skquad-system",
                "--include-crds",
            ],
            cwd=REPO_ROOT,
        ).stdout

        expected_fragments = [
            "kind: CustomResourceDefinition",
            "name: agents.skquad.io",
            "credentialSecret:",
            "virtualKeySecret:",
            "desiredActive:",
            "name: skquad-api-server",
            "name: SKQUAD_K8S_ENABLED",
            "name: SKQUAD_AGENT_IMAGE",
            "name: SKQUAD_CONTROL_PLANE_URL",
            "name: SKQUAD_LLM_GATEWAY_URL",
            "name: SKQUAD_LITELLM_ADMIN_URL",
            "name: SKQUAD_LITELLM_MASTER_KEY",
            "name: skquad-llm-gateway",
            "name: DATABASE_URL",
            "name: LITELLM_MASTER_KEY",
            "name: SKQUAD_GATEWAY_CALLBACK_TOKEN",
            "name: skquad-operator",
            "name: SKQUAD_AGENT_TASK_POLL_INTERVAL_SECONDS",
            "name: SKQUAD_AGENT_INBOX_POLL_INTERVAL_SECONDS",
            "name: SKQUAD_AGENT_TASK_TIMEOUT_SECONDS",
            "name: SKQUAD_AGENT_MAX_LLM_STEPS",
            "name: skquad-web",
            "name: SKQUAD_API_BASE_URL",
            "name: NEXT_PUBLIC_SKQUAD_API_BASE_URL",
        ]
        for fragment in expected_fragments:
            with self.subTest(fragment=fragment):
                self.assertIn(fragment, rendered)

    def run_command(
        self,
        args: list[str],
        *,
        cwd: Path,
        env: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess[str]:
        merged_env = os.environ.copy()
        if env:
            merged_env.update(env)
        result = subprocess.run(
            args,
            cwd=cwd,
            env=merged_env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        if result.returncode != 0:
            self.fail(
                f"{' '.join(args)} failed in {cwd} with exit {result.returncode}\n"
                f"{result.stdout}"
            )
        return result


if __name__ == "__main__":
    unittest.main()
