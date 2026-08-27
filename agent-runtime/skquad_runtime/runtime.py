"""Agent runtime bootstrap and health surface.

The full task loop lands later. This module defines the runtime contract the
operator already provides: environment configuration plus read-only mounted
credential directories.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path
from typing import Mapping


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
