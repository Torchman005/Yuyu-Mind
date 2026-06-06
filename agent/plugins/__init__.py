from __future__ import annotations

import importlib.util
import json
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable


PluginAction = Callable[[dict[str, Any]], dict[str, Any]]
PLUGIN_SCHEMA_VERSION = "mochi.plugin.v1"


@dataclass(frozen=True)
class PluginSpec:
    name: str
    plugin_dir: Path
    display_name: str
    description: str
    version: str
    author: str
    enabled: bool
    permissions: list[str]
    metadata: dict[str, Any]
    config: dict[str, Any]
    actions: dict[str, PluginAction]


_PLUGINS: dict[str, PluginSpec] = {}


def register_plugin(spec: PluginSpec) -> None:
    if not spec.name:
        raise ValueError("plugin name cannot be empty")
    _PLUGINS[spec.name] = spec


def load_builtin_plugins() -> None:
    load_plugins()


def load_plugins(root: Path | None = None) -> None:
    _PLUGINS.clear()
    plugin_root = root or Path(__file__).resolve().parent
    for metadata_path in sorted(plugin_root.glob("*/plugin.json")):
        try:
            register_plugin(load_plugin_from_manifest(metadata_path))
        except Exception as error:
            print(f"[plugin] failed to load {metadata_path}: {error}", flush=True)


def load_plugin_from_manifest(metadata_path: Path) -> PluginSpec:
    metadata = json.loads(metadata_path.read_text(encoding="utf-8-sig"))
    if metadata.get("schemaVersion") != PLUGIN_SCHEMA_VERSION:
        raise ValueError(f"unsupported plugin schema: {metadata.get('schemaVersion')}")

    name = str(metadata.get("name", "")).strip()
    if not name:
        raise ValueError("plugin metadata is missing name")
    if name != metadata_path.parent.name:
        raise ValueError(f"plugin name {name!r} must match directory name {metadata_path.parent.name!r}")

    actions_metadata = metadata.get("actions", [])
    if not isinstance(actions_metadata, list) or not actions_metadata:
        raise ValueError(f"plugin {name} must declare at least one action")

    entry = str(metadata.get("entry", "main.py")).strip() or "main.py"
    module_path = metadata_path.parent / entry
    if not module_path.exists():
        raise ValueError(f"plugin entry was not found: {entry}")
    config = load_plugin_config(metadata_path.parent, metadata)

    module = load_plugin_module(name, module_path)
    exported_actions = getattr(module, "ACTIONS", None)
    if not isinstance(exported_actions, dict):
        raise ValueError(f"plugin {name} entry must export ACTIONS")

    actions: dict[str, PluginAction] = {}
    for action_metadata in actions_metadata:
        if not isinstance(action_metadata, dict):
            raise ValueError(f"plugin {name} action metadata must be an object")
        action_name = str(action_metadata.get("name", "")).strip()
        if not action_name:
            raise ValueError(f"plugin {name} has an action without name")
        handler = exported_actions.get(action_name)
        if not callable(handler):
            raise ValueError(f"plugin {name} action {action_name} has no callable handler")
        actions[action_name] = handler

    permissions = metadata.get("permissions", [])
    if not isinstance(permissions, list):
        permissions = []

    return PluginSpec(
        name=name,
        plugin_dir=metadata_path.parent,
        display_name=str(metadata.get("displayName", name)).strip() or name,
        description=str(metadata.get("description", "")).strip(),
        version=str(metadata.get("version", "0.0.0")).strip() or "0.0.0",
        author=str(metadata.get("author", "")).strip(),
        enabled=bool(metadata.get("enabled", True)),
        permissions=[str(item).strip() for item in permissions if str(item).strip()],
        metadata=metadata,
        config=config,
        actions=actions,
    )


def load_plugin_config(plugin_dir: Path, metadata: dict[str, Any]) -> dict[str, Any]:
    defaults = metadata.get("defaultConfig", {})
    if not isinstance(defaults, dict):
        defaults = {}

    config_path = plugin_dir / "config.json"
    if not config_path.exists():
        return dict(defaults)

    loaded = json.loads(config_path.read_text(encoding="utf-8-sig"))
    if not isinstance(loaded, dict):
        raise ValueError(f"plugin config must be a JSON object: {config_path}")
    return merge_config(defaults, loaded)


def merge_config(defaults: dict[str, Any], loaded: dict[str, Any]) -> dict[str, Any]:
    merged = dict(defaults)
    for key, value in loaded.items():
        if isinstance(value, dict) and isinstance(merged.get(key), dict):
            merged[key] = merge_config(merged[key], value)
        else:
            merged[key] = value
    return merged


def load_plugin_module(name: str, module_path: Path) -> Any:
    module_name = f"plugins.{name}._runtime"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    if spec is None or spec.loader is None:
        raise ValueError(f"cannot load plugin module: {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def list_plugins() -> list[dict[str, Any]]:
    plugins: list[dict[str, Any]] = []
    for spec in sorted(_PLUGINS.values(), key=lambda item: item.name):
        manifest = dict(spec.metadata)
        manifest["actions"] = [
            action for action in manifest.get("actions", []) if isinstance(action, dict) and action.get("name") in spec.actions
        ]
        manifest["loadedActions"] = sorted(spec.actions.keys())
        manifest["config"] = redact_config(spec.config, manifest.get("configSchema", {}))
        plugins.append(manifest)
    return plugins


def redact_config(config: dict[str, Any], schema: Any) -> dict[str, Any]:
    if not isinstance(config, dict):
        return {}
    properties = schema.get("properties", {}) if isinstance(schema, dict) else {}
    redacted: dict[str, Any] = {}
    for key, value in config.items():
        property_schema = properties.get(key, {}) if isinstance(properties, dict) else {}
        if isinstance(property_schema, dict) and property_schema.get("secret"):
            redacted[key] = "********" if str(value).strip() else ""
        elif isinstance(value, dict):
            redacted[key] = redact_config(value, property_schema)
        else:
            redacted[key] = value
    return redacted


def invoke_plugin(name: str, action: str, payload: dict[str, Any] | None = None) -> dict[str, Any]:
    spec = _PLUGINS.get(name)
    if spec is None:
        raise KeyError(f"unknown plugin: {name}")
    if not spec.enabled:
        raise RuntimeError(f"plugin is disabled: {name}")
    handler = spec.actions.get(action)
    if handler is None:
        raise KeyError(f"unknown plugin action: {name}.{action}")
    request = dict(payload or {})
    request.setdefault("config", load_plugin_config(spec.plugin_dir, spec.metadata))
    return handler(request)
