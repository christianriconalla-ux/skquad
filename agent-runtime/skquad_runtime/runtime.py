"""Agent runtime bootstrap and health surface.

The full task loop lands later. This module defines the runtime contract the
operator already provides: environment configuration plus read-only mounted
credential directories.
"""

from __future__ import annotations

import os
import json
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Mapping
from urllib import error, request


DEFAULT_CREDENTIALS_DIR = Path("/var/run/skquad/credentials")
DEFAULT_AGENT_CREDENTIAL_PATH = DEFAULT_CREDENTIALS_DIR / "agent"
DEFAULT_VIRTUAL_KEY_PATH = DEFAULT_CREDENTIALS_DIR / "llm-gateway"


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

    @property
    def missing_required(self) -> list[str]:
        missing: list[str] = []
        if not self.agent_id:
            missing.append("SKQUAD_AGENT_ID")
        if not self.squad_id:
            missing.append("SKQUAD_SQUAD_ID")
        return missing


@dataclass(frozen=True)
class BootstrapStatus:
    ready: bool
    agent_id: str
    squad_id: str
    missing_required: list[str]
    credential_loaded: bool
    virtual_key_loaded: bool


@dataclass(frozen=True)
class RuntimeTask:
    id: str
    squad_id: str
    title: str
    description: str
    status: str
    assignee_agent_id: str


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
    )


def bootstrap_status(config: BootstrapConfig) -> BootstrapStatus:
    missing = config.missing_required
    credential = read_secret_value(config.agent_credential_path)
    virtual_key = read_secret_value(config.virtual_key_path)
    return BootstrapStatus(
        ready=not missing and credential is not None,
        agent_id=config.agent_id,
        squad_id=config.squad_id,
        missing_required=missing,
        credential_loaded=credential is not None,
        virtual_key_loaded=virtual_key is not None,
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

    def claim_task(self) -> RuntimeTask | None:
        payload = self._json("POST", "/api/v1/agents/me/tasks/claim", None, allow_empty=True)
        if payload is None:
            return None
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
        }

    return app


def main() -> None:
    import uvicorn

    host = os.environ.get("SKQUAD_RUNTIME_HOST", "0.0.0.0")
    port = int(os.environ.get("SKQUAD_RUNTIME_PORT", "8080"))
    uvicorn.run(create_app(), host=host, port=port)
