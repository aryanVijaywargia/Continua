# Continua Python SDK

Python SDK for building AI agents with Continua.

## Installation

```bash
pip install continua
```

## Quick Start

```python
from continua import Agent, AgentConfig, AgentContext, tool


class MyAgent(Agent):
    @tool(description="Search the web")
    async def search(self, query: str) -> str:
        # Your search implementation
        return f"Results for: {query}"

    async def run(self, context: AgentContext, input_data: str) -> str:
        result = await self.search(input_data)
        return result


# Create and run the agent
agent = MyAgent(AgentConfig(name="my-agent"))
```

## Documentation

See the [full documentation](https://docs.continua.dev) for more details.
