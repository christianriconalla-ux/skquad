"""Agent runtime bootstrap, health surface, and task loop."""

from __future__ import annotations

import os
import json
import inspect
import logging
import threading
from dataclasses import dataclass
from pathlib import Path
from time import sleep as default_sleep
from typing import Callable, Mapping, Protocol
from urllib import error, request


DEFAULT_CREDENTIALS_DIR = Path("/var/run/skquad/credentials")
DEFAULT_AGENT_CREDENTIAL_PATH = DEFAULT_CREDENTIALS_DIR / "agent"
DEFAULT_VIRTUAL_KEY_PATH = DEFAULT_CREDENTIALS_DIR / "llm-gateway"
LOGGER = logging.getLogger(__name__)


@dataclass(frozen=True)
class BootstrapConfig:
    agent_id: str
    squad_id: str
    role: str
    default_provider_id: str
    idle_timeout: str
    credentials_dir: Path
    agent_credential_path: Path
    virtual_key_path: Path
    control_plane_url: str
    llm_gateway_url: str
    task_loop_enabled: bool

    @property
    def missing_required(self) -> list[str]:
        missing: list[str] = []
        if not self.agent_id:
            missing.append("SKQUAD_AGENT_ID")
        if not self.squad_id:
            missing.append("SKQUAD_SQUAD_ID")
        if self.task_loop_enabled:
            if not self.default_provider_id:
                missing.append("SKQUAD_DEFAULT_PROVIDER_ID")
            if not self.control_plane_url:
                missing.append("SKQUAD_CONTROL_PLANE_URL")
            if not self.llm_gateway_url:
                missing.append("SKQUAD_LLM_GATEWAY_URL")
        return missing


@dataclass(frozen=True)
class BootstrapStatus:
    ready: bool
    agent_id: str
    squad_id: str
    missing_required: list[str]
    credential_loaded: bool
    virtual_key_loaded: bool
    task_loop_enabled: bool


@dataclass(frozen=True)
class RuntimeTask:
    id: str
    squad_id: str
    title: str
    description: str
    status: str
    assignee_agent_id: str


@dataclass(frozen=True)
class RuntimeResource:
    resource_type: str
    resource_id: str
    name: str
    description: str
    endpoint: str
    manifest: Mapping[str, object]


@dataclass(frozen=True)
class TaskResult:
    status: str = "in-review"
    summary: str = ""


@dataclass(frozen=True)
class ToolCall:
    id: str
    name: str
    arguments: Mapping[str, object]


@dataclass(frozen=True)
class ToolResult:
    content: str
    ok: bool = True


class TaskHandler(Protocol):
    def handle_task(self, task: RuntimeTask, config: BootstrapConfig) -> TaskResult:
        ...


class RuntimePlugin(Protocol):
    name: str

    def tools(self) -> list[Mapping[str, object]]:
        ...

    def invoke(self, call: ToolCall, config: BootstrapConfig) -> ToolResult | str:
        ...


def load_bootstrap_config(environ: Mapping[str, str] | None = None) -> BootstrapConfig:
    env = os.environ if environ is None else environ
    credentials_dir = Path(env.get("SKQUAD_CREDENTIALS_DIR", str(DEFAULT_CREDENTIALS_DIR)))
    return BootstrapConfig(
        agent_id=env.get("SKQUAD_AGENT_ID", ""),
        squad_id=env.get("SKQUAD_SQUAD_ID", ""),
        role=env.get("SKQUAD_AGENT_ROLE", ""),
        default_provider_id=env.get("SKQUAD_DEFAULT_PROVIDER_ID", ""),
        idle_timeout=env.get("SKQUAD_IDLE_TIMEOUT", ""),
        credentials_dir=credentials_dir,
        agent_credential_path=Path(
            env.get("SKQUAD_AGENT_CREDENTIAL_PATH", str(DEFAULT_AGENT_CREDENTIAL_PATH))
        ),
        virtual_key_path=Path(
            env.get("SKQUAD_LLM_GATEWAY_VIRTUAL_KEY_PATH", str(DEFAULT_VIRTUAL_KEY_PATH))
        ),
        control_plane_url=env.get("SKQUAD_CONTROL_PLANE_URL", ""),
        llm_gateway_url=env.get("SKQUAD_LLM_GATEWAY_URL", ""),
        task_loop_enabled=env_bool(env, "SKQUAD_TASK_LOOP_ENABLED", True),
    )


def bootstrap_status(config: BootstrapConfig) -> BootstrapStatus:
    missing = config.missing_required
    credential = read_secret_value(config.agent_credential_path)
    virtual_key = read_secret_value(config.virtual_key_path)
    secret_ready = credential is not None and (
        not config.task_loop_enabled or virtual_key is not None
    )
    return BootstrapStatus(
        ready=not missing and secret_ready,
        agent_id=config.agent_id,
        squad_id=config.squad_id,
        missing_required=missing,
        credential_loaded=credential is not None,
        virtual_key_loaded=virtual_key is not None,
        task_loop_enabled=config.task_loop_enabled,
    )


def read_secret_value(path: Path, preferred_keys: tuple[str, ...] = ("token", "credential", "api_key", "value")) -> str | None:
    if path.is_file():
        return path.read_text(encoding="utf-8").strip()
    if not path.is_dir():
        return None
    for key in preferred_keys:
        value = read_secret_value(path / key, preferred_keys=())
        if value:
            return value
    for child in sorted(path.iterdir()):
        if child.name.startswith(".."):
            continue
        value = read_secret_value(child, preferred_keys=())
        if value:
            return value
    return None


class ControlPlaneClient:
    def __init__(
        self,
        base_url: str,
        agent_id: str,
        credential: str,
        opener: Callable[[request.Request], object] | None = None,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.agent_id = agent_id
        self.credential = credential
        self._opener = opener or request.urlopen

    @classmethod
    def from_bootstrap(cls, config: BootstrapConfig) -> "ControlPlaneClient":
        credential = read_secret_value(config.agent_credential_path)
        if credential is None:
            raise RuntimeError("agent credential is not loaded")
        if not config.control_plane_url:
            raise RuntimeError("SKQUAD_CONTROL_PLANE_URL is required")
        return cls(config.control_plane_url, config.agent_id, credential)

    def heartbeat(self, status: str) -> dict[str, object]:
        return self._json("POST", "/api/v1/agents/me/heartbeat", {"status": status})

    def list_tasks(self) -> list[RuntimeTask]:
        payload = self._json("GET", "/api/v1/agents/me/tasks", None)
        return [runtime_task(item) for item in payload]

    def list_resources(self) -> list[RuntimeResource]:
        payload = self._json("GET", "/api/v1/agents/me/resources", None)
        return [runtime_resource(item) for item in payload]

    def claim_task(self) -> RuntimeTask | None:
        payload = self._json("POST", "/api/v1/agents/me/tasks/claim", None, allow_empty=True)
        if payload is None:
            return None
        return runtime_task(payload)

    def start_task(self, task_id: str) -> RuntimeTask:
        payload = self._json("POST", f"/api/v1/agents/me/tasks/{task_id}/start", None)
        return runtime_task(payload)

    def complete_task(self, task_id: str, status: str = "in-review") -> RuntimeTask:
        payload = self._json("POST", f"/api/v1/agents/me/tasks/{task_id}/complete", {"status": status})
        return runtime_task(payload)

    def block_task(self, task_id: str) -> RuntimeTask:
        payload = self._json("POST", f"/api/v1/agents/me/tasks/{task_id}/block", None)
        return runtime_task(payload)

    def _json(self, method: str, path: str, body: object | None, allow_empty: bool = False):
        data = None
        headers = {
            "Authorization": f"Bearer {self.credential}",
            "X-Skquad-Agent-ID": self.agent_id,
            "Accept": "application/json",
        }
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        req = request.Request(self.base_url + path, data=data, headers=headers, method=method)
        try:
            with self._opener(req) as response:
                if response.status == 204 and allow_empty:
                    return None
                payload = response.read()
        except error.HTTPError as exc:
            if exc.code == 204 and allow_empty:
                return None
            raise RuntimeError(f"control-plane request failed: {exc.code}") from exc
        if not payload and allow_empty:
            return None
        return json.loads(payload.decode("utf-8"))


def runtime_task(payload: Mapping[str, object]) -> RuntimeTask:
    return RuntimeTask(
        id=str(payload.get("id", "")),
        squad_id=str(payload.get("squad_id", "")),
        title=str(payload.get("title", "")),
        description=str(payload.get("description", "")),
        status=str(payload.get("status", "")),
        assignee_agent_id=str(payload.get("assignee_agent_id", "")),
    )


def runtime_resource(payload: Mapping[str, object]) -> RuntimeResource:
    manifest = payload.get("manifest") or {}
    if not isinstance(manifest, Mapping):
        manifest = {}
    return RuntimeResource(
        resource_type=str(payload.get("resource_type", "")),
        resource_id=str(payload.get("resource_id", "")),
        name=str(payload.get("name", "")),
        description=str(payload.get("description", "")),
        endpoint=str(payload.get("endpoint", "")),
        manifest=manifest,
    )


class LiteLLMTaskHandler:
    def __init__(
        self,
        plugins: list[RuntimePlugin] | None = None,
        resources: list[RuntimeResource] | None = None,
        completion: Callable[..., object] | None = None,
        model: str | None = None,
        max_steps: int = 8,
        discover_resources: bool = True,
    ) -> None:
        self.plugins = plugins or []
        self.resources = resources
        self._completion = completion
        self.model = model
        self.max_steps = max_steps
        self.discover_resources = discover_resources

    def handle_task(self, task: RuntimeTask, config: BootstrapConfig) -> TaskResult:
        virtual_key = read_secret_value(config.virtual_key_path)
        if virtual_key is None:
            raise RuntimeError("LLM gateway virtual key is not loaded")
        if not config.llm_gateway_url:
            raise RuntimeError("SKQUAD_LLM_GATEWAY_URL is required")
        model = self.model or config.default_provider_id
        if not model:
            raise RuntimeError("SKQUAD_DEFAULT_PROVIDER_ID is required")

        messages: list[dict[str, object]] = [
            {
                "role": "system",
                "content": system_prompt(config, self.available_resources(config)),
            },
            {
                "role": "user",
                "content": task_prompt(task),
            },
        ]
        tools = self.tool_schemas()
        completion = self.completion()
        last_content = ""

        for _ in range(self.max_steps):
            completion_kwargs: dict[str, object] = {
                "model": model,
                "messages": messages,
                "api_base": config.llm_gateway_url.rstrip("/"),
                "api_key": virtual_key,
            }
            if tools:
                completion_kwargs["tools"] = tools
            response = completion(**completion_kwargs)
            message = first_message(response)
            content = str(message_value(message, "content") or "")
            last_content = content
            tool_calls = parse_tool_calls(message)
            messages.append(assistant_message(content, tool_calls))
            if not tool_calls:
                return TaskResult(status=status_from_content(content), summary=content)
            for call in tool_calls:
                result = self.invoke_tool(call, config)
                messages.append(
                    {
                        "role": "tool",
                        "tool_call_id": call.id,
                        "name": call.name,
                        "content": result.content,
                    }
                )

        return TaskResult(status="in-review", summary=last_content)

    def completion(self) -> Callable[..., object]:
        if self._completion is not None:
            return self._completion
        try:
            from litellm import completion
        except ModuleNotFoundError as exc:
            raise RuntimeError("litellm is required for the default task handler") from exc
        return completion

    def tool_schemas(self) -> list[Mapping[str, object]]:
        schemas: list[Mapping[str, object]] = []
        for plugin in self.plugins:
            schemas.extend(plugin.tools())
        return schemas

    def available_resources(self, config: BootstrapConfig) -> list[RuntimeResource]:
        if self.resources is not None:
            return self.resources
        if not self.discover_resources:
            return []
        self.resources = ControlPlaneClient.from_bootstrap(config).list_resources()
        return self.resources

    def invoke_tool(self, call: ToolCall, config: BootstrapConfig) -> ToolResult:
        plugin = next((item for item in self.plugins if item.name == call.name), None)
        if plugin is None:
            return ToolResult(content=f"tool {call.name!r} is not available", ok=False)
        try:
            result = plugin.invoke(call, config)
            if inspect.isawaitable(result):
                import asyncio

                result = asyncio.run(result)
            if isinstance(result, ToolResult):
                return result
            return ToolResult(content=str(result))
        except Exception as exc:
            return ToolResult(content=f"tool {call.name!r} failed: {exc}", ok=False)


def system_prompt(config: BootstrapConfig, resources: list[RuntimeResource] | None = None) -> str:
    role = config.role or "skquad agent"
    prompt = (
        f"You are {role}. Work on exactly one assigned task at a time. "
        "Use available tools when they materially help. Return a concise result. "
        "Use 'SKQUAD_STATUS: done' only when the task is fully complete; use "
        "'SKQUAD_STATUS: blocked' when you cannot proceed."
    )
    if resources:
        prompt += "\n\nGranted resources:\n" + "\n".join(resource_prompt_line(item) for item in resources)
    return prompt


def resource_prompt_line(resource: RuntimeResource) -> str:
    bits = [resource.resource_type, resource.name]
    if resource.description:
        bits.append(resource.description)
    if resource.endpoint:
        bits.append(resource.endpoint)
    package_ref = resource.manifest.get("package_ref")
    if package_ref:
        bits.append(f"package={package_ref}")
    return "- " + " | ".join(bits)


def task_prompt(task: RuntimeTask) -> str:
    description = task.description.strip() or "(no description)"
    return f"Task: {task.title}\n\nDescription:\n{description}"


def first_message(response: object) -> object:
    choices = object_value(response, "choices") or []
    if not choices:
        raise RuntimeError("LLM response did not include choices")
    first = choices[0]
    message = object_value(first, "message")
    if message is None:
        raise RuntimeError("LLM response choice did not include a message")
    return message


def parse_tool_calls(message: object) -> list[ToolCall]:
    calls = message_value(message, "tool_calls") or []
    parsed: list[ToolCall] = []
    for index, raw_call in enumerate(calls):
        function = object_value(raw_call, "function") or {}
        raw_arguments = object_value(function, "arguments") or "{}"
        try:
            arguments = json.loads(raw_arguments) if isinstance(raw_arguments, str) else raw_arguments
        except json.JSONDecodeError:
            arguments = {"_raw": raw_arguments}
        if not isinstance(arguments, Mapping):
            arguments = {"value": arguments}
        parsed.append(
            ToolCall(
                id=str(object_value(raw_call, "id") or f"tool-call-{index}"),
                name=str(object_value(function, "name") or ""),
                arguments=arguments,
            )
        )
    return parsed


def assistant_message(content: str, tool_calls: list[ToolCall]) -> dict[str, object]:
    message: dict[str, object] = {"role": "assistant", "content": content}
    if tool_calls:
        message["tool_calls"] = [
            {
                "id": call.id,
                "type": "function",
                "function": {"name": call.name, "arguments": json.dumps(call.arguments)},
            }
            for call in tool_calls
        ]
    return message


def status_from_content(content: str) -> str:
    normalized = content.lower()
    if "skquad_status: blocked" in normalized:
        return "blocked"
    if "skquad_status: done" in normalized:
        return "done"
    return "in-review"


def message_value(message: object, key: str) -> object | None:
    return object_value(message, key)


def object_value(item: object, key: str) -> object | None:
    if isinstance(item, Mapping):
        return item.get(key)
    return getattr(item, key, None)


def poll_once(config: BootstrapConfig, client: ControlPlaneClient | None = None) -> RuntimeTask | None:
    status = bootstrap_status(config)
    if not status.ready:
        return None
    control_plane = client or ControlPlaneClient.from_bootstrap(config)
    task = control_plane.claim_task()
    if task is None:
        control_plane.heartbeat("idle")
        return None
    control_plane.heartbeat("busy")
    return task


def run_task_once(
    config: BootstrapConfig,
    handler: TaskHandler,
    client: ControlPlaneClient | None = None,
) -> RuntimeTask | None:
    status = bootstrap_status(config)
    if not status.ready:
        return None
    control_plane = client or ControlPlaneClient.from_bootstrap(config)
    task = control_plane.claim_task()
    if task is None:
        control_plane.heartbeat("idle")
        return None
    control_plane.heartbeat("busy")
    try:
        result = handler.handle_task(task, config)
    except Exception:
        final_task = control_plane.block_task(task.id)
        control_plane.heartbeat("idle")
        return final_task
    if result.status not in ("in-review", "done", "blocked"):
        final_task = control_plane.block_task(task.id)
        control_plane.heartbeat("idle")
        return final_task
    if result.status == "blocked":
        final_task = control_plane.block_task(task.id)
    else:
        final_task = control_plane.complete_task(task.id, result.status)
    control_plane.heartbeat("idle")
    return final_task


def run_task_loop(
    config: BootstrapConfig,
    handler: TaskHandler,
    client: ControlPlaneClient | None = None,
    poll_interval_seconds: float = 5.0,
    stop_event: object | None = None,
    sleeper: Callable[[float], None] = default_sleep,
) -> None:
    while not stop_requested(stop_event):
        try:
            run_task_once(config, handler, client)
        except Exception:
            LOGGER.exception("agent task loop iteration failed")
        sleeper(poll_interval_seconds)


def stop_requested(stop_event: object | None) -> bool:
    return bool(stop_event is not None and getattr(stop_event, "is_set")())


def env_bool(environ: Mapping[str, str], name: str, default: bool) -> bool:
    value = environ.get(name)
    if value is None:
        return default
    return value.strip().lower() in ("1", "true", "yes", "on")


def create_app(config: BootstrapConfig | None = None):
    from fastapi import FastAPI, Response, status

    app = FastAPI(title="skquad agent runtime", version="0.1.0")

    @app.get("/healthz")
    def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/readyz")
    def readyz(response: Response) -> dict[str, object]:
        status_result = bootstrap_status(config or load_bootstrap_config())
        if not status_result.ready:
            response.status_code = status.HTTP_503_SERVICE_UNAVAILABLE
        return {
            "ready": status_result.ready,
            "agent_id": status_result.agent_id,
            "squad_id": status_result.squad_id,
            "missing_required": status_result.missing_required,
            "credential_loaded": status_result.credential_loaded,
            "virtual_key_loaded": status_result.virtual_key_loaded,
            "task_loop_enabled": status_result.task_loop_enabled,
        }

    return app


def main() -> None:
    import uvicorn

    config = load_bootstrap_config()
    if config.task_loop_enabled:
        poll_interval = float(os.environ.get("SKQUAD_TASK_POLL_INTERVAL_SECONDS", "5"))
        worker = threading.Thread(
            target=run_task_loop,
            args=(config, LiteLLMTaskHandler()),
            kwargs={"poll_interval_seconds": poll_interval},
            daemon=True,
        )
        worker.start()

    host = os.environ.get("SKQUAD_RUNTIME_HOST", "0.0.0.0")
    port = int(os.environ.get("SKQUAD_RUNTIME_PORT", "8080"))
    uvicorn.run(create_app(config), host=host, port=port)
