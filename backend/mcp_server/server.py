"""
JumpServer MCP Server Entry Point

This module initializes and runs the MCP server, registering all resources,
tools, and prompts from their respective modules.
"""

from .app import mcp
import dotenv

dotenv.load_dotenv()
# load_dotenv(find_dotenv(str(BASE_DIR / ".env")))


def register_all():
    """
    Explicitly register all MCP resources, tools, and prompts.
    
    This function imports all registration modules, which will register
    their components via decorators (@mcp.resource, @mcp.tool, @mcp.prompt).
    
    Raises:
        ImportError: If any registration module fails to import.
    """
    # Import order matters - resources should be registered first
    from . import resources  # Register MCP resources
    from . import login  # Register login tool (accessible when not authenticated)
    from . import tools  # Register MCP tools
    from . import users  # Register user-specific MCP tools
    from . import prompts  # Register MCP prompts
    from . import profile  # Register MCP profile


# Register all components
register_all()


if __name__ == "__main__":
    mcp.run(transport="http", port=8000, host="0.0.0.0")