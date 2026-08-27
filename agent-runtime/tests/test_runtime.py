import json
import tempfile
import unittest
from pathlib import Path

from skquad_runtime.runtime import (
    ControlPlaneClient,
    LiteLLMTaskHandler,
    RuntimeResource,
    RuntimeTask,
    TaskResult,
    ToolResult,
    bootstrap_status,
    create_app,
    load_bootstrap_config,
    poll_once,
    read_secret_value,
    run_task_once,
)


class RuntimeBootstrapTest(unittest.TestCase):
    def test_load_bootstrap_config_from_environment(self):
        config = load_bootstrap_config(
            {
                "SKQUAD_AGENT_ID": "agent-1",
                "SKQUAD_SQUAD_ID": "squad-1",
                "SKQUAD_AGENT_ROLE": "coder",
                "SKQUAD_DEFAULT_PROVIDER_ID": "provider-1",
                "SKQUAD_DEFAULT_MODEL": "openai/gpt-4o-mini",
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
        self.assertEqual(config.default_provider_id, "provider-1")
        self.assertEqual(config.default_model, "openai/gpt-4o-mini")
        self.assertEqual(config.agent_credential_path, Path("/tmp/credentials/agent"))
        self.assertEqual(config.virtual_key_path, Path("/tmp/credentials/gateway"))
        self.assertTrue(config.task_loop_enabled)

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
                [
                    "SKQUAD_AGENT_ID",
                    "SKQUAD_SQUAD_ID",
                    "SKQUAD_DEFAULT_MODEL",
                    "SKQUAD_CONTROL_PLANE_URL",
                    "SKQUAD_LLM_GATEWAY_URL",
            ],
        )

    def test_bootstrap_status_ready_with_required_task_loop_config_and_secrets(self):
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
                    "SKQUAD_DEFAULT_PROVIDER_ID": "provider-1",
                    "SKQUAD_DEFAULT_MODEL": "model-1",
                    "SKQUAD_AGENT_CREDENTIAL_PATH": str(credential_dir),
                    "SKQUAD_LLM_GATEWAY_VIRTUAL_KEY_PATH": str(virtual_key_dir),
                    "SKQUAD_CONTROL_PLANE_URL": "http://control-plane",
                    "SKQUAD_LLM_GATEWAY_URL": "http://gateway",
                }
            )

            status = bootstrap_status(config)

            self.assertTrue(status.ready)
            self.assertTrue(status.credential_loaded)
            self.assertTrue(status.virtual_key_loaded)
            self.assertTrue(status.task_loop_enabled)

    def test_bootstrap_status_not_ready_without_task_loop_virtual_key(self):
        with tempfile.TemporaryDirectory() as tmp:
            credential = Path(tmp) / "agent"
            credential.write_text("agent-token", encoding="utf-8")
            config = load_bootstrap_config(
                {
                    "SKQUAD_AGENT_ID": "agent-1",
                    "SKQUAD_SQUAD_ID": "squad-1",
                    "SKQUAD_DEFAULT_MODEL": "model-1",
                    "SKQUAD_AGENT_CREDENTIAL_PATH": str(credential),
                    "SKQUAD_LLM_GATEWAY_VIRTUAL_KEY_PATH": str(Path(tmp) / "missing-gateway"),
                    "SKQUAD_CONTROL_PLANE_URL": "http://control-plane",
                    "SKQUAD_LLM_GATEWAY_URL": "http://gateway",
                }
            )

            status = bootstrap_status(config)

            self.assertFalse(status.ready)
            self.assertTrue(status.credential_loaded)
            self.assertFalse(status.virtual_key_loaded)

    def test_bootstrap_status_ready_without_virtual_key_when_task_loop_disabled(self):
        with tempfile.TemporaryDirectory() as tmp:
            credential = Path(tmp) / "agent"
            credential.write_text("agent-token", encoding="utf-8")
            config = load_bootstrap_config(
                {
                    "SKQUAD_AGENT_ID": "agent-1",
                    "SKQUAD_SQUAD_ID": "squad-1",
                    "SKQUAD_AGENT_CREDENTIAL_PATH": str(credential),
                    "SKQUAD_TASK_LOOP_ENABLED": "false",
                }
            )

            status = bootstrap_status(config)

            self.assertTrue(status.ready)
            self.assertTrue(status.credential_loaded)
            self.assertFalse(status.virtual_key_loaded)
            self.assertFalse(status.task_loop_enabled)

    def test_control_plane_client_sends_agent_auth_headers(self):
        calls = []

        def opener(req):
            calls.append(req)
            return FakeResponse(200, b'{"id":"task-1","squad_id":"squad-1","title":"T","description":"","status":"in-progress","assignee_agent_id":"agent-1"}')

        client = ControlPlaneClient("http://control-plane", "agent-1", "credential", opener=opener)

        task = client.claim_task()

        self.assertEqual(task.id, "task-1")
        self.assertEqual(calls[0].full_url, "http://control-plane/api/v1/agents/me/tasks/claim")
        self.assertEqual(calls[0].headers["Authorization"], "Bearer credential")
        self.assertEqual(calls[0].headers["X-skquad-agent-id"], "agent-1")

    def test_control_plane_client_claim_handles_no_content(self):
        client = ControlPlaneClient(
            "http://control-plane",
            "agent-1",
            "credential",
            opener=lambda _req: FakeResponse(204, b""),
        )

        self.assertIsNone(client.claim_task())

    def test_control_plane_client_lists_runtime_resources(self):
        calls = []

        def opener(req):
            calls.append(req)
            return FakeResponse(
                200,
                b'[{"resource_type":"tool","resource_id":"tool-1","name":"echo",'
                b'"description":"Echo messages","endpoint":"plugin://echo",'
                b'"manifest":{"package_ref":"builtin://echo"}}]',
            )

        client = ControlPlaneClient("http://control-plane", "agent-1", "credential", opener=opener)

        resources = client.list_resources()

        self.assertEqual(resources[0].resource_type, "tool")
        self.assertEqual(resources[0].manifest["package_ref"], "builtin://echo")
        self.assertEqual(calls[0].full_url, "http://control-plane/api/v1/agents/me/resources")

    def test_poll_once_reports_idle_without_task(self):
        with tempfile.TemporaryDirectory() as tmp:
            credential = Path(tmp) / "agent"
            credential.write_text("credential", encoding="utf-8")
            config = load_bootstrap_config(
                {
                    "SKQUAD_AGENT_ID": "agent-1",
                    "SKQUAD_SQUAD_ID": "squad-1",
                    "SKQUAD_AGENT_CREDENTIAL_PATH": str(credential),
                    "SKQUAD_TASK_LOOP_ENABLED": "false",
                }
            )
            client = FakeControlPlaneClient(claimed_task=None)

            task = poll_once(config, client)

            self.assertIsNone(task)
            self.assertEqual(client.heartbeats, ["idle"])

    def test_poll_once_reports_busy_with_claimed_task(self):
        with tempfile.TemporaryDirectory() as tmp:
            credential = Path(tmp) / "agent"
            credential.write_text("credential", encoding="utf-8")
            config = load_bootstrap_config(
                {
                    "SKQUAD_AGENT_ID": "agent-1",
                    "SKQUAD_SQUAD_ID": "squad-1",
                    "SKQUAD_AGENT_CREDENTIAL_PATH": str(credential),
                    "SKQUAD_TASK_LOOP_ENABLED": "false",
                }
            )
            client = FakeControlPlaneClient(claimed_task=fake_task("task-1"))

            task = poll_once(config, client)

            self.assertEqual(task.id, "task-1")
            self.assertEqual(client.heartbeats, ["busy"])

    def test_run_task_once_completes_successful_handler_result(self):
        with tempfile.TemporaryDirectory() as tmp:
            config = ready_config(tmp)
            client = FakeControlPlaneClient(claimed_task=fake_task("task-1"))
            handler = StaticTaskHandler(TaskResult(status="done"))

            task = run_task_once(config, handler, client)

            self.assertEqual(task.id, "task-1")
            self.assertEqual(client.completed, [("task-1", "done")])
            self.assertEqual(client.blocked, [])
            self.assertEqual(client.heartbeats, ["busy", "idle"])

    def test_run_task_once_blocks_when_handler_raises(self):
        with tempfile.TemporaryDirectory() as tmp:
            config = ready_config(tmp)
            client = FakeControlPlaneClient(claimed_task=fake_task("task-1"))
            handler = RaisingTaskHandler()

            task = run_task_once(config, handler, client)

            self.assertEqual(task.id, "task-1")
            self.assertEqual(client.completed, [])
            self.assertEqual(client.blocked, ["task-1"])
            self.assertEqual(client.heartbeats, ["busy", "idle"])

    def test_run_task_once_blocks_invalid_handler_status(self):
        with tempfile.TemporaryDirectory() as tmp:
            config = ready_config(tmp)
            client = FakeControlPlaneClient(claimed_task=fake_task("task-1"))
            handler = StaticTaskHandler(TaskResult(status="not-real"))

            task = run_task_once(config, handler, client)

            self.assertEqual(task.id, "task-1")
            self.assertEqual(client.completed, [])
            self.assertEqual(client.blocked, ["task-1"])

    def test_litellm_handler_calls_gateway_and_returns_done_status(self):
        with tempfile.TemporaryDirectory() as tmp:
            config = ready_config(tmp)
            virtual_key = Path(tmp) / "llm-gateway"
            virtual_key.write_text("virtual-key", encoding="utf-8")
            config = load_bootstrap_config(
                {
                    "SKQUAD_AGENT_ID": "agent-1",
                    "SKQUAD_SQUAD_ID": "squad-1",
                    "SKQUAD_AGENT_CREDENTIAL_PATH": str(Path(tmp) / "agent"),
                    "SKQUAD_LLM_GATEWAY_VIRTUAL_KEY_PATH": str(virtual_key),
                    "SKQUAD_CONTROL_PLANE_URL": "http://control-plane",
                    "SKQUAD_LLM_GATEWAY_URL": "http://gateway",
                    "SKQUAD_DEFAULT_PROVIDER_ID": "provider-1",
                    "SKQUAD_DEFAULT_MODEL": "model-1",
                }
            )
            calls = []

            def completion(**kwargs):
                calls.append(kwargs)
                return fake_completion("SKQUAD_STATUS: done\nImplemented.")

            handler = LiteLLMTaskHandler(completion=completion, discover_resources=False)

            result = handler.handle_task(fake_task("task-1"), config)

            self.assertEqual(result.status, "done")
            self.assertEqual(calls[0]["model"], "model-1")
            self.assertEqual(calls[0]["api_base"], "http://gateway")
            self.assertEqual(calls[0]["api_key"], "virtual-key")

    def test_litellm_handler_falls_back_to_legacy_provider_env_for_model(self):
        with tempfile.TemporaryDirectory() as tmp:
            virtual_key = Path(tmp) / "llm-gateway"
            virtual_key.write_text("virtual-key", encoding="utf-8")
            config = load_bootstrap_config(
                {
                    "SKQUAD_AGENT_ID": "agent-1",
                    "SKQUAD_SQUAD_ID": "squad-1",
                    "SKQUAD_AGENT_CREDENTIAL_PATH": str(Path(tmp) / "agent"),
                    "SKQUAD_LLM_GATEWAY_VIRTUAL_KEY_PATH": str(virtual_key),
                    "SKQUAD_LLM_GATEWAY_URL": "http://gateway",
                    "SKQUAD_DEFAULT_PROVIDER_ID": "legacy-model-alias",
                }
            )
            calls = []

            def completion(**kwargs):
                calls.append(kwargs)
                return fake_completion("Ready for review.")

            handler = LiteLLMTaskHandler(completion=completion, discover_resources=False)

            result = handler.handle_task(fake_task("task-1"), config)

            self.assertEqual(result.status, "in-review")
            self.assertEqual(calls[0]["model"], "legacy-model-alias")

    def test_litellm_handler_includes_runtime_resources_in_prompt(self):
        with tempfile.TemporaryDirectory() as tmp:
            virtual_key = Path(tmp) / "llm-gateway"
            virtual_key.write_text("virtual-key", encoding="utf-8")
            config = load_bootstrap_config(
                {
                    "SKQUAD_AGENT_ID": "agent-1",
                    "SKQUAD_SQUAD_ID": "squad-1",
                    "SKQUAD_AGENT_CREDENTIAL_PATH": str(Path(tmp) / "agent"),
                    "SKQUAD_LLM_GATEWAY_VIRTUAL_KEY_PATH": str(virtual_key),
                    "SKQUAD_LLM_GATEWAY_URL": "http://gateway",
                    "SKQUAD_DEFAULT_MODEL": "model-1",
                }
            )
            calls = []

            def completion(**kwargs):
                calls.append(kwargs)
                return fake_completion("Ready for review.")

            handler = LiteLLMTaskHandler(
                resources=[
                    RuntimeResource(
                        resource_type="tool",
                        resource_id="tool-1",
                        name="echo",
                        description="Echo messages",
                        endpoint="plugin://echo",
                        manifest={"package_ref": "builtin://echo"},
                    )
                ],
                completion=completion,
            )

            result = handler.handle_task(fake_task("task-1"), config)

            self.assertEqual(result.status, "in-review")
            system_message = calls[0]["messages"][0]["content"]
            self.assertIn("Granted resources:", system_message)
            self.assertIn("tool | echo | Echo messages | plugin://echo | package=builtin://echo", system_message)

    def test_litellm_handler_invokes_plugin_tool_calls(self):
        with tempfile.TemporaryDirectory() as tmp:
            config = ready_config(tmp)
            virtual_key = Path(tmp) / "llm-gateway"
            virtual_key.write_text("virtual-key", encoding="utf-8")
            config = load_bootstrap_config(
                {
                    "SKQUAD_AGENT_ID": "agent-1",
                    "SKQUAD_SQUAD_ID": "squad-1",
                    "SKQUAD_AGENT_CREDENTIAL_PATH": str(Path(tmp) / "agent"),
                    "SKQUAD_LLM_GATEWAY_VIRTUAL_KEY_PATH": str(virtual_key),
                    "SKQUAD_LLM_GATEWAY_URL": "http://gateway",
                    "SKQUAD_DEFAULT_MODEL": "model-1",
                }
            )
            plugin = EchoPlugin()
            calls = []

            def completion(**kwargs):
                calls.append(kwargs)
                if len(calls) == 1:
                    return fake_tool_completion("call-1", "echo", {"message": "hello"})
                return fake_completion("SKQUAD_STATUS: done\nTool observed.")

            handler = LiteLLMTaskHandler(plugins=[plugin], completion=completion, discover_resources=False)

            result = handler.handle_task(fake_task("task-1"), config)

            self.assertEqual(result.status, "done")
            self.assertEqual(plugin.calls, [{"message": "hello"}])
            self.assertEqual(calls[0]["tools"][0]["function"]["name"], "echo")
            tool_messages = [item for item in calls[1]["messages"] if item["role"] == "tool"]
            self.assertEqual(tool_messages[-1]["content"], "echo: hello")


class FakeResponse:
    def __init__(self, status, payload):
        self.status = status
        self._payload = payload

    def __enter__(self):
        return self

    def __exit__(self, _exc_type, _exc, _tb):
        return False

    def read(self):
        return self._payload


class FakeControlPlaneClient:
    def __init__(self, claimed_task):
        self.claimed_task = claimed_task
        self.heartbeats = []
        self.completed = []
        self.blocked = []

    def claim_task(self):
        return self.claimed_task

    def heartbeat(self, status):
        self.heartbeats.append(status)
        return {}

    def complete_task(self, task_id, status="in-review"):
        self.completed.append((task_id, status))
        return fake_task(task_id, status=status)

    def block_task(self, task_id):
        self.blocked.append(task_id)
        return fake_task(task_id, status="blocked")


class StaticTaskHandler:
    def __init__(self, result):
        self.result = result

    def handle_task(self, _task, _config):
        return self.result


class RaisingTaskHandler:
    def handle_task(self, _task, _config):
        raise RuntimeError("handler failed")


class EchoPlugin:
    name = "echo"

    def __init__(self):
        self.calls = []

    def tools(self):
        return [
            {
                "type": "function",
                "function": {
                    "name": "echo",
                    "description": "Echo a message.",
                    "parameters": {
                        "type": "object",
                        "properties": {"message": {"type": "string"}},
                        "required": ["message"],
                    },
                },
            }
        ]

    def invoke(self, call, _config):
        self.calls.append(dict(call.arguments))
        return ToolResult(content=f"echo: {call.arguments['message']}")


def fake_task(task_id, status="in-progress"):
    return RuntimeTask(
        id=task_id,
        squad_id="squad-1",
        title="Task",
        description="",
        status=status,
        assignee_agent_id="agent-1",
    )


def fake_completion(content):
    return {"choices": [{"message": {"content": content}}]}


def fake_tool_completion(call_id, name, arguments):
    return {
        "choices": [
            {
                "message": {
                    "content": "",
                    "tool_calls": [
                        {
                            "id": call_id,
                            "type": "function",
                            "function": {
                                "name": name,
                                "arguments": json.dumps(arguments),
                            },
                        }
                    ],
                }
            }
        ]
    }


def ready_config(tmp):
    credential = Path(tmp) / "agent"
    credential.write_text("credential", encoding="utf-8")
    return load_bootstrap_config(
        {
            "SKQUAD_AGENT_ID": "agent-1",
            "SKQUAD_SQUAD_ID": "squad-1",
            "SKQUAD_AGENT_CREDENTIAL_PATH": str(credential),
            "SKQUAD_TASK_LOOP_ENABLED": "false",
        }
    )


if __name__ == "__main__":
    unittest.main()
