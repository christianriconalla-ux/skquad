import tempfile
import unittest
from pathlib import Path

from skquad_runtime.runtime import (
    bootstrap_status,
    create_app,
    load_bootstrap_config,
    read_secret_value,
)


class RuntimeBootstrapTest(unittest.TestCase):
    def test_load_bootstrap_config_from_environment(self):
        config = load_bootstrap_config(
            {
                "SKQUAD_AGENT_ID": "agent-1",
                "SKQUAD_SQUAD_ID": "squad-1",
                "SKQUAD_AGENT_ROLE": "coder",
                "SKQUAD_DEFAULT_PROVIDER_ID": "provider-1",
                "SKQUAD_IDLE_TIMEOUT": "300s",
                "SKQUAD_CREDENTIALS_DIR": "/tmp/credentials",
                "SKQUAD_AGENT_CREDENTIAL_PATH": "/tmp/credentials/agent",
                "SKQUAD_LLM_GATEWAY_VIRTUAL_KEY_PATH": "/tmp/credentials/gateway",
                "SKQUAD_CONTROL_PLANE_URL": "http://api",
                "SKQUAD_LLM_GATEWAY_URL": "http://gateway",
            }
        )

        self.assertEqual(config.agent_id, "agent-1")
        self.assertEqual(config.squad_id, "squad-1")
        self.assertEqual(config.role, "coder")
        self.assertEqual(config.agent_credential_path, Path("/tmp/credentials/agent"))
        self.assertEqual(config.virtual_key_path, Path("/tmp/credentials/gateway"))

    def test_read_secret_value_prefers_known_keys(self):
        with tempfile.TemporaryDirectory() as tmp:
            secret_dir = Path(tmp)
            (secret_dir / "other").write_text("other-value", encoding="utf-8")
            (secret_dir / "token").write_text("token-value\n", encoding="utf-8")

            self.assertEqual(read_secret_value(secret_dir), "token-value")

    def test_readyz_reports_missing_required_config(self):
        try:
            from fastapi.testclient import TestClient
        except ModuleNotFoundError:
            self.skipTest("fastapi is not installed")

        client = TestClient(create_app(load_bootstrap_config({})))

        response = client.get("/readyz")

        self.assertEqual(response.status_code, 503)
        self.assertEqual(
            response.json()["missing_required"],
            ["SKQUAD_AGENT_ID", "SKQUAD_SQUAD_ID"],
        )

    def test_bootstrap_status_ready_with_required_config_and_credential(self):
        with tempfile.TemporaryDirectory() as tmp:
            credential_dir = Path(tmp) / "agent"
            credential_dir.mkdir()
            (credential_dir / "token").write_text("agent-token", encoding="utf-8")
            virtual_key_dir = Path(tmp) / "llm-gateway"
            virtual_key_dir.mkdir()
            (virtual_key_dir / "token").write_text("gateway-token", encoding="utf-8")
            config = load_bootstrap_config(
                {
                    "SKQUAD_AGENT_ID": "agent-1",
                    "SKQUAD_SQUAD_ID": "squad-1",
                    "SKQUAD_AGENT_CREDENTIAL_PATH": str(credential_dir),
                    "SKQUAD_LLM_GATEWAY_VIRTUAL_KEY_PATH": str(virtual_key_dir),
                }
            )

            status = bootstrap_status(config)

            self.assertTrue(status.ready)
            self.assertTrue(status.credential_loaded)
            self.assertTrue(status.virtual_key_loaded)


if __name__ == "__main__":
    unittest.main()
