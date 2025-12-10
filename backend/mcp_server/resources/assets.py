"""
Asset-related MCP resources for JumpServer.

Exposes read-only metadata such as supported categories and types.
"""

from __future__ import annotations

from typing import Any, Dict

from ..app import mcp
from ..utils import safe_request

# Supported asset categories
CATEGORIES = [
    "host",
    "device",
    "database",
    "web",
    "cloud",
    "ds",
]

# Asset types mapping by category
ASSET_TYPES = {
    "host": [
        {"value": "linux", "label": "Linux"},
        {"value": "windows", "label": "Windows"},
        {"value": "macos", "label": "macOS"},
        {"value": "unix", "label": "Unix"},
        {"value": "other", "label": "Other"},
    ],
    "device": [
        {"value": "cisco", "label": "Cisco"},
        {"value": "huawei", "label": "Huawei"},
        {"value": "h3c", "label": "H3C"},
        {"value": "juniper", "label": "Juniper"},
        {"value": "tp_link", "label": "TP-Link"},
        {"value": "general", "label": "General"},
        {"value": "switch", "label": "Switch"},
        {"value": "router", "label": "Router"},
        {"value": "firewall", "label": "Firewall"},
    ],
    "database": [
        {"value": "mysql", "label": "MySQL"},
        {"value": "mariadb", "label": "MariaDB"},
        {"value": "postgresql", "label": "PostgreSQL"},
        {"value": "oracle", "label": "Oracle"},
        {"value": "sqlserver", "label": "SQLServer"},
        {"value": "db2", "label": "DB2"},
        {"value": "dameng", "label": "Dameng"},
        {"value": "clickhouse", "label": "ClickHouse"},
        {"value": "mongodb", "label": "MongoDB"},
        {"value": "redis", "label": "Redis"},
    ],
    "cloud": [
        {"value": "public", "label": "Public cloud"},
        {"value": "private", "label": "Private cloud"},
        {"value": "k8s", "label": "Kubernetes"},
    ],
    "ds": [
        {"value": "general", "label": "General"},
        {"value": "windows_ad", "label": "Windows Active Directory"},
    ],
    "web": [
        # Add web types as needed
    ],
}


@mcp.resource("data://asset-categories")
def get_asset_categories() -> Dict[str, Any]:
    """
    Return the supported asset categories.
    """
    return {
        "categories": CATEGORIES,
    }


@mcp.resource("data://asset-types")
def get_asset_types() -> Dict[str, Any]:
    """
    Return all asset types organized by category.
    """
    return {
        "types": ASSET_TYPES,
    }


@mcp.resource("data://asset-types/{category}")
def get_asset_types_by_category(category: str) -> Dict[str, Any]:
    """
    Return asset types for a specific category.
    
    Args:
        category: Asset category (e.g., "host", "device", "database")
    """
    if category not in CATEGORIES:
        return {
            "error": f"Invalid category '{category}'. Valid categories: {CATEGORIES}",
        }
    
    return {
        "category": category,
        "types": ASSET_TYPES.get(category, []),
    }
