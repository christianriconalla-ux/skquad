"""skquad agent runtime.

The thin custom harness that runs inside each agent pod. It owns the agent's
lifecycle, task-scoped context, the core work loop, and the plugin interface.
It is model-agnostic (via LiteLLM) and extensible (via plugins).

See docs/agent-runtime.md.
"""

from .runtime import (
    BootstrapConfig,
    ControlPlaneClient,
    LiteLLMTaskHandler,
    RuntimeMemory,
    RuntimeResource,
    RuntimeTask,
    RuntimeTaskContext,
    RuntimePlugin,
    TaskResult,
    ToolCall,
    ToolResult,
    bootstrap_status,
    create_app,
    load_bootstrap_config,
    load_runtime_plugins,
    poll_once,
    run_task_loop,
    run_task_once,
)

__all__ = [
    "BootstrapConfig",
    "ControlPlaneClient",
    "LiteLLMTaskHandler",
    "RuntimeMemory",
    "RuntimeResource",
    "RuntimeTask",
    "RuntimeTaskContext",
    "RuntimePlugin",
    "TaskResult",
    "ToolCall",
    "ToolResult",
    "bootstrap_status",
    "create_app",
    "load_bootstrap_config",
    "load_runtime_plugins",
    "poll_once",
    "run_task_loop",
    "run_task_once",
]

__version__ = "0.1.0"
