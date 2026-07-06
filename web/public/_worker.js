const sessions = [
  {
    "created_at": "2026-06-03T00:42:25.175485+05:30",
    "external_id": "demo-sdk-public-demo-incident",
    "id": "48cffb69-9c27-45f9-9db3-1a0f5b66f50f",
    "metadata": {
      "agent_mode": "openai",
      "demo": true,
      "run_id": "public-demo"
    },
    "name": "Real agent demo: demo-sdk-public-demo-incident",
    "trace_count": 1
  },
  {
    "created_at": "2026-06-03T00:42:11.870516+05:30",
    "external_id": "demo-sdk-public-demo-research",
    "id": "cfb33ecf-2148-489f-84e7-a9f366dc2377",
    "metadata": {
      "agent_mode": "openai",
      "demo": true,
      "run_id": "public-demo"
    },
    "name": "Real agent demo: demo-sdk-public-demo-research",
    "trace_count": 2
  }
];

const traces = [
  {
    "ended_at": "2026-06-03T00:42:26.250695+05:30",
    "error_count": 1,
    "id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4",
    "name": "failing_agent",
    "session_external_id": "demo-sdk-public-demo-incident",
    "session_id": "48cffb69-9c27-45f9-9db3-1a0f5b66f50f",
    "started_at": "2026-06-03T00:42:24.479453+05:30",
    "status": "FAILED",
    "total_cost_usd": 0,
    "total_tokens_in": 38,
    "total_tokens_out": 80
  },
  {
    "ended_at": "2026-06-03T00:42:24.45114+05:30",
    "error_count": 0,
    "id": "88e6b423-cb06-47dc-a649-dd9737b47ee5",
    "name": "code_review_agent",
    "session_external_id": "demo-sdk-public-demo-research",
    "session_id": "cfb33ecf-2148-489f-84e7-a9f366dc2377",
    "started_at": "2026-06-03T00:42:19.523311+05:30",
    "status": "COMPLETED",
    "total_cost_usd": 0,
    "total_tokens_in": 604,
    "total_tokens_out": 329
  },
  {
    "ended_at": "2026-06-03T00:42:19.427109+05:30",
    "error_count": 0,
    "id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7",
    "name": "research_agent",
    "session_external_id": "demo-sdk-public-demo-research",
    "session_id": "cfb33ecf-2148-489f-84e7-a9f366dc2377",
    "started_at": "2026-06-03T00:42:10.857567+05:30",
    "status": "COMPLETED",
    "total_cost_usd": 0,
    "total_tokens_in": 1009,
    "total_tokens_out": 472
  }
];

const traceDetails = new Map(Object.entries({
  "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4": {
    "ended_at": "2026-06-03T00:42:26.250695+05:30",
    "error_count": 1,
    "id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4",
    "name": "failing_agent",
    "session_external_id": "demo-sdk-public-demo-incident",
    "session_id": "48cffb69-9c27-45f9-9db3-1a0f5b66f50f",
    "started_at": "2026-06-03T00:42:24.479453+05:30",
    "status": "FAILED",
    "total_cost_usd": 0,
    "total_tokens_in": 38,
    "total_tokens_out": 80,
    "trace_id": "13f51fd2-053a-48af-ac50-1ae97bc56087"
  },
  "88e6b423-cb06-47dc-a649-dd9737b47ee5": {
    "ended_at": "2026-06-03T00:42:24.45114+05:30",
    "error_count": 0,
    "id": "88e6b423-cb06-47dc-a649-dd9737b47ee5",
    "name": "code_review_agent",
    "output": {
      "status": "completed",
      "target": "sdks/python/src/continua/trace.py"
    },
    "session_external_id": "demo-sdk-public-demo-research",
    "session_id": "cfb33ecf-2148-489f-84e7-a9f366dc2377",
    "started_at": "2026-06-03T00:42:19.523311+05:30",
    "status": "COMPLETED",
    "total_cost_usd": 0,
    "total_tokens_in": 604,
    "total_tokens_out": 329,
    "trace_id": "ac89ef24-ac9f-4f9a-87d5-15922848e10b"
  },
  "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7": {
    "ended_at": "2026-06-03T00:42:19.427109+05:30",
    "error_count": 0,
    "id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7",
    "name": "research_agent",
    "output": {
      "query": "What are the benefits of AI observability?",
      "retrieved_sources": 4,
      "status": "completed"
    },
    "session_external_id": "demo-sdk-public-demo-research",
    "session_id": "cfb33ecf-2148-489f-84e7-a9f366dc2377",
    "started_at": "2026-06-03T00:42:10.857567+05:30",
    "status": "COMPLETED",
    "total_cost_usd": 0,
    "total_tokens_in": 1009,
    "total_tokens_out": 472,
    "trace_id": "26eb1cf0-3191-45c0-939b-776a1cb8d85e"
  }
}));

const traceSpansByTrace = new Map(Object.entries({
  "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4": {
    "spans": [
      {
        "ended_at": "2026-06-03T00:42:26.234691+05:30",
        "id": "820c88bd-d4ec-4655-9f17-7c296b6f2443",
        "input": [
          {
            "content": "You are an incident triage agent. Return JSON with first_check and rationale.",
            "role": "system"
          },
          {
            "content": "Plan the first dependency check for a stalled agent workflow.",
            "role": "user"
          }
        ],
        "input_truncated": false,
        "kind": "LLM",
        "latency_ms": 1755,
        "model": "gpt-4.1-mini-2025-04-14",
        "name": "initial_step",
        "output": {
          "first_check": "Verify the status and connectivity of all external dependencies and services that the agent workflow relies on.",
          "rationale": "A stalled agent workflow often results from issues with external dependencies such as APIs, databases, or third-party services. Checking their status and connectivity first helps identify if the stall is due to an unavailable or unresponsive dependency, allowing for targeted troubleshooting."
        },
        "output_truncated": false,
        "provider": "openai",
        "span_id": "ddf7a09e-f915-4094-bef8-7747b87954f8",
        "started_at": "2026-06-03T00:42:24.479532+05:30",
        "status": "COMPLETED",
        "tokens_in": 38,
        "tokens_out": 80,
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      },
      {
        "ended_at": "2026-06-03T00:42:26.250674+05:30",
        "error_message": "Dependency health check failed",
        "id": "479d221c-a44a-4b32-bf6d-74fe63ded467",
        "input": {
          "target": "http://127.0.0.1:9/continua-demo-health"
        },
        "input_truncated": false,
        "kind": "TOOL",
        "latency_ms": 16,
        "name": "dependency_health_probe",
        "output": {
          "duration_ms": 13.86,
          "error": "[Errno 61] Connection refused",
          "error_type": "ConnectError",
          "status": "failed",
          "target": "http://127.0.0.1:9/continua-demo-health"
        },
        "output_truncated": false,
        "span_id": "39b893e2-5960-4323-bb5a-e21009dc8ed0",
        "started_at": "2026-06-03T00:42:26.234781+05:30",
        "status": "FAILED",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      }
    ]
  },
  "88e6b423-cb06-47dc-a649-dd9737b47ee5": {
    "spans": [
      {
        "ended_at": "2026-06-03T00:42:19.550583+05:30",
        "id": "ad6a8500-16d5-490b-9ae9-370e335b2ac0",
        "input_truncated": false,
        "kind": "CHAIN",
        "latency_ms": 27,
        "name": "code_analysis_chain",
        "output_truncated": false,
        "span_id": "030fa1a6-efd1-4d28-bd15-f2005c0f7f02",
        "started_at": "2026-06-03T00:42:19.523347+05:30",
        "status": "COMPLETED",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "ended_at": "2026-06-03T00:42:19.524046+05:30",
        "id": "ff7f27ae-e3cf-4bbb-af8c-fb3373e76124",
        "input": {
          "path": "sdks/python/src/continua/trace.py"
        },
        "input_truncated": false,
        "kind": "TOOL",
        "latency_ms": 1,
        "name": "parse_python_source",
        "output": {
          "class_count": 0,
          "classes": [],
          "function_count": 14,
          "functions": [
            "get_current_trace",
            "__init__",
            "set_input",
            "set_output",
            "set_metadata",
            "__enter__",
            "__exit__",
            "_send_trace_start",
            "_send_trace_end",
            "trace",
            "my_agent",
            "another_agent"
          ],
          "line_count": 217,
          "path": "sdks/python/src/continua/trace.py",
          "sample": "\"\"\"Trace context and decorator for tracing agent executions.\"\"\"\n\nfrom __future__ import annotations\n\nimport functools\nimport uuid\nfrom collections.abc import Callable\nfrom contextvars import ContextVar\nfrom datetime import datetime, timezone\nfrom typing import Any, TypeVar\n\nfrom .client import _get_client_if_initialized\nfrom .session import get_current_session\n\n# Context variable for the current trace\n_current_trace: ContextVar[TraceContext | None] = ContextVar(\n    \"current_trace\", default=None\n)\n\nT = TypeVar(\"T\")\n\n\ndef get_current_trace() -> TraceContext | None:\n    \"\"\"Get the current trace context, if any.\"\"\"\n    return _current_trace.get()\n\n\nclass TraceContext:\n    \"\"\"Context manager for a trace execution.\n\n    Tracks the lifecycle of a trace and automatically sends it to the server.\n\n    Example:\n        with TraceContext(name=\"my_agent\") as trace:\n            trace.set_input({\"query\": \"...\"})\n            # do work\n            trace.set_output({\"response\": \"...\"})\n    \"\"\"\n\n    def __init__(\n        self,\n        name: str,\n        *,\n        session_id: str | None = None,\n        user_id: str | None = None,\n        tags: list[str] | None = None,\n        metadata: dict[str, Any] | None = None,\n    ) -> None:\n        \"\"\"Create a new trace context.\n\n        Args:\n            name: Name of the trace\n            session_id: Optional session identifier. If not provided and a session\n                        context is active, inherits from the session.\n            user_id: Optional user identifier. If not provided and a session\n                     context is active with a user_id, inherits from the session.\n            tags: Optional list of tags\n            metadata: Optional metadata dictionary\n        \"\"\"\n        self.trace_id = str(uuid.uuid4())\n        self.name = n"
        },
        "output_truncated": false,
        "parent_span_id": "030fa1a6-efd1-4d28-bd15-f2005c0f7f02",
        "span_id": "b302775f-7769-44ff-9301-39c1ebef53c1",
        "started_at": "2026-06-03T00:42:19.523364+05:30",
        "status": "COMPLETED",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "ended_at": "2026-06-03T00:42:19.550576+05:30",
        "id": "19f6dfb2-eeee-4a4c-a19d-e76c5b30646a",
        "input": {
          "path": "sdks/python/src/continua/trace.py"
        },
        "input_truncated": false,
        "kind": "TOOL",
        "latency_ms": 27,
        "name": "local_quality_scan",
        "output": {
          "long_lines": 0,
          "sends_trace_start_before_user_input": false,
          "uses_contextvars": true
        },
        "output_truncated": false,
        "parent_span_id": "030fa1a6-efd1-4d28-bd15-f2005c0f7f02",
        "span_id": "11854168-97f4-40d4-a2e8-de094f2a7d0e",
        "started_at": "2026-06-03T00:42:19.524066+05:30",
        "status": "COMPLETED",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "ended_at": "2026-06-03T00:42:24.450246+05:30",
        "id": "38eb3a04-7ce7-4f3b-90e9-b859e023e24f",
        "input": [
          {
            "content": "You are reviewing a Python SDK tracing module. Return JSON with keys summary, findings, and score.",
            "role": "system"
          },
          {
            "content": "{\"static_analysis\": {\"path\": \"sdks/python/src/continua/trace.py\", \"line_count\": 217, \"function_count\": 14, \"class_count\": 0, \"functions\": [\"get_current_trace\", \"__init__\", \"set_input\", \"set_output\", \"set_metadata\", \"__enter__\", \"__exit__\", \"_send_trace_start\", \"_send_trace_end\", \"trace\", \"my_agent\", \"another_agent\"], \"classes\": [], \"sample\": \"\\\"\\\"\\\"Trace context and decorator for tracing agent executions.\\\"\\\"\\\"\\n\\nfrom __future__ import annotations\\n\\nimport functools\\nimport uuid\\nfrom collections.abc import Callable\\nfrom contextvars import ContextVar\\nfrom datetime import datetime, timezone\\nfrom typing import Any, TypeVar\\n\\nfrom .client import _get_client_if_initialized\\nfrom .session import get_current_session\\n\\n# Context variable for the current trace\\n_current_trace: ContextVar[TraceContext | None] = ContextVar(\\n    \\\"current_trace\\\", default=None\\n)\\n\\nT = TypeVar(\\\"T\\\")\\n\\n\\ndef get_current_trace() -> TraceContext | None:\\n    \\\"\\\"\\\"Get the current trace context, if any.\\\"\\\"\\\"\\n    return _current_trace.get()\\n\\n\\nclass TraceContext:\\n    \\\"\\\"\\\"Context manager for a trace execution.\\n\\n    Tracks the lifecycle of a trace and automatically sends it to the server.\\n\\n    Example:\\n        with TraceContext(name=\\\"my_agent\\\") as trace:\\n            trace.set_input({\\\"query\\\": \\\"...\\\"})\\n            # do work\\n            trace.set_output({\\\"response\\\": \\\"...\\\"})\\n    \\\"\\\"\\\"\\n\\n    def __init__(\\n        self,\\n        name: str,\\n        *,\\n        session_id: str | None = None,\\n        user_id: str | None = None,\\n        tags: list[str] | None = None,\\n        metadata: dict[str, Any] | None = None,\\n    ) -> None:\\n        \\\"\\\"\\\"Create a new trace context.\\n\\n        Args:\\n            name: Name of the trace\\n            session_id: Optional session identifier. If not provided and a session\\n                        context is active, inherits from the session.\\n            user_id: Optional user identifier. If not provided and a session\\n                     context is active with a user_id, inherits from the session.\\n            tags: Optional list of tags\\n            metadata: Optional metadata dictionary\\n        \\\"\\\"\\\"\\n        self.trace_id = str(uuid.uuid4())\\n        self.name = n\"}, \"quality_scan\": {\"long_lines\": 0, \"uses_contextvars\": true, \"sends_trace_start_before_user_input\": false}}",
            "role": "user"
          }
        ],
        "input_truncated": false,
        "kind": "LLM",
        "latency_ms": 4900,
        "model": "gpt-4.1-mini-2025-04-14",
        "name": "generate_review",
        "output": {
          "findings": [
            {
              "impact": "This could lead to incomplete or inaccurate trace data if the trace start event is delayed until after user input is set.",
              "issue": "Trace start not sent before user input",
              "recommendation": "Consider sending the trace start event immediately upon trace context creation or entry to ensure full trace coverage."
            },
            {
              "impact": "The module is simple and focused, which is good for maintainability.",
              "issue": "No classes other than TraceContext",
              "recommendation": "No action needed."
            },
            {
              "impact": "Proper use of contextvars allows trace context to be maintained correctly across async boundaries.",
              "issue": "Use of contextvars",
              "recommendation": "No action needed."
            },
            {
              "impact": "Code readability is maintained.",
              "issue": "No long lines detected",
              "recommendation": "No action needed."
            }
          ],
          "score": 8.5,
          "summary": "The Python SDK tracing module provides a TraceContext class for managing trace lifecycles and a get_current_trace function to access the current trace context. It uses contextvars to maintain trace state across asynchronous calls. The TraceContext supports setting inputs, outputs, metadata, and automatically sends trace data to a server. The module is well-structured with 14 functions and no classes other than TraceContext. There are no long lines detected, and contextvars usage is appropriate. However, the trace start is not sent before user input, which may affect trace accuracy."
        },
        "output_truncated": false,
        "provider": "openai",
        "span_id": "692bf577-2028-4630-b104-87e3eefbfb68",
        "started_at": "2026-06-03T00:42:19.550604+05:30",
        "status": "COMPLETED",
        "tokens_in": 604,
        "tokens_out": 329,
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      }
    ]
  },
  "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7": {
    "spans": [
      {
        "ended_at": "2026-06-03T00:42:14.330518+05:30",
        "id": "ac99f487-c389-41ac-bdce-26c64e959b50",
        "input": [
          {
            "content": "You are a concise research agent. Return a JSON object with keys plan, chosen_focus, and alternatives.",
            "role": "system"
          },
          {
            "content": "Create a research plan for: What are the benefits of AI observability?",
            "role": "user"
          }
        ],
        "input_truncated": false,
        "kind": "LLM",
        "latency_ms": 3473,
        "model": "gpt-4.1-mini-2025-04-14",
        "name": "plan_research",
        "output": {
          "alternatives": [
            "Focus on technical benefits of AI observability such as improved model performance and debugging.",
            "Explore business impacts like increased trust and compliance due to AI observability.",
            "Investigate challenges and limitations alongside benefits to provide a balanced view."
          ],
          "chosen_focus": "Identify and analyze the primary benefits of AI observability through literature review and case studies.",
          "plan": [
            "Define AI observability and its key components.",
            "Identify the primary benefits of AI observability from academic papers, industry reports, and expert articles.",
            "Analyze case studies where AI observability has been implemented to understand practical advantages.",
            "Compare AI observability benefits with traditional monitoring approaches.",
            "Summarize findings highlighting technical, operational, and business benefits."
          ]
        },
        "output_truncated": false,
        "provider": "openai",
        "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
        "started_at": "2026-06-03T00:42:10.857923+05:30",
        "status": "COMPLETED",
        "tokens_in": 47,
        "tokens_out": 165,
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "ended_at": "2026-06-03T00:42:14.336087+05:30",
        "id": "0a50014a-bf7a-4249-83da-18bc5372a511",
        "input": {
          "limit": 4,
          "query": "What are the benefits of AI observability?",
          "root": "docs-site/concepts"
        },
        "input_truncated": false,
        "kind": "TOOL",
        "latency_ms": 6,
        "name": "docs_search",
        "output": {
          "matches": [
            {
              "path": "docs-site/concepts/projects-and-auth.mdx",
              "score": 43,
              "snippet": "--- title: \"Projects & auth\" description: \"How projects, API keys, Auth0 operator login, and demo mode fit together.\" --- A **project** is the unit of multi-tenancy in Continua. Every trace, span, session, and ingest batch is scoped to exactly one project. Every authenticated request must resolve to a project. ## The three ways a request gets authenticated | Mechanism | Used by | How project_id is resolved | | --- | --- | --- | | **API key** (`X-API-Key`) | SDKs, ingest, REST integrators | Server hashes the key (SHA-256), looks it up in `projects`, injects the owning project into context | | **Auth0 bearer** (operator JWT) | Debugger UI when `AUTH0_ENABLED=true` | The UI picks a project expl"
            },
            {
              "path": "docs-site/concepts/cost-and-tokens.mdx",
              "score": 37,
              "snippet": "--- title: \"Cost & tokens\" description: \"How per-span token counts and cost values roll up into trace and session totals.\" --- Cost and token counts are first-class in Continua. The SDK lets you attach them to any span; the platform rolls them up to the trace level and the debugger surfaces them in multiple places. ## The data path 1. Inside a span, the SDK records counts via `set_tokens` and `set_cost`: ```python with span(\"call_model\", kind=\"llm\") as s: answer = call_model(prompt) s.set_tokens(prompt=120, completion=48) s.set_cost(0.0023) ``` 2. The ingest path persists these on the span row. 3. A rollup River worker recomputes per-trace totals (sum of cost, sum of token counts per bucket,"
            },
            {
              "path": "docs-site/concepts/data-model.mdx",
              "score": 37,
              "snippet": "--- title: \"Data model\" description: \"Projects, sessions, traces, spans, events: the persisted shapes that back the debugger.\" --- ```mermaid erDiagram projects ||--o{ ingest_batches : \"owns\" projects ||--o{ sessions : \"owns\" projects ||--o{ traces : \"owns\" projects ||--o{ spans : \"owns\" projects ||--o{ span_events : \"owns\" sessions ||--o{ traces : \"groups\" traces ||--o{ spans : \"contains\" traces ||--o{ span_events : \"contains\" spans ||--o{ span_events : \"emits via span_id\" projects { uuid id PK string name string api_key_hash timestamptz created_at } ingest_batches { uuid id PK uuid project_id FK string batch_key string status int trace_count int span_count } sessions { uuid id PK uuid proj"
            },
            {
              "path": "docs-site/concepts/events.mdx",
              "score": 37,
              "snippet": "--- title: \"Events\" description: \"Semantic event types that explain what happened inside a span: state changes, decisions, effects, waits, and more.\" --- Continua events are **debugger-facing observability signals**. They explain *what happened* during trace execution. They are not replay primitives, checkpoints, or durable workflow state machines. ```mermaid flowchart LR subgraph Operational log metric end subgraph Failure error exception end subgraph Semantic state_change decision effect wait end subgraph Milestone message snapshot_marker end subgraph Domain custom end ``` ## Choosing an event type - **`state_change`**: observable state transitions you want shown as `old \u2192 new` in the debu"
            }
          ]
        },
        "output_truncated": false,
        "span_id": "5bfb0245-5353-4c22-9e75-55b3185a700f",
        "started_at": "2026-06-03T00:42:14.330584+05:30",
        "status": "COMPLETED",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "ended_at": "2026-06-03T00:42:19.427057+05:30",
        "id": "88fb2aa9-c259-401a-ad9c-1b70f6279211",
        "input": [
          {
            "content": "You synthesize retrieved project docs for a demo trace. Return concise JSON with keys synthesis, risks, and confidence.",
            "role": "system"
          },
          {
            "content": "{\"query\": \"What are the benefits of AI observability?\", \"plan\": {\"plan\": [\"Define AI observability and its key components.\", \"Identify the primary benefits of AI observability from academic papers, industry reports, and expert articles.\", \"Analyze case studies where AI observability has been implemented to understand practical advantages.\", \"Compare AI observability benefits with traditional monitoring approaches.\", \"Summarize findings highlighting technical, operational, and business benefits.\"], \"chosen_focus\": \"Identify and analyze the primary benefits of AI observability through literature review and case studies.\", \"alternatives\": [\"Focus on technical benefits of AI observability such as improved model performance and debugging.\", \"Explore business impacts like increased trust and compliance due to AI observability.\", \"Investigate challenges and limitations alongside benefits to provide a balanced view.\"]}, \"retrieved_docs\": [{\"path\": \"docs-site/concepts/projects-and-auth.mdx\", \"score\": 43, \"snippet\": \"--- title: \\\"Projects & auth\\\" description: \\\"How projects, API keys, Auth0 operator login, and demo mode fit together.\\\" --- A **project** is the unit of multi-tenancy in Continua. Every trace, span, session, and ingest batch is scoped to exactly one project. Every authenticated request must resolve to a project. ## The three ways a request gets authenticated | Mechanism | Used by | How project_id is resolved | | --- | --- | --- | | **API key** (`X-API-Key`) | SDKs, ingest, REST integrators | Server hashes the key (SHA-256), looks it up in `projects`, injects the owning project into context | | **Auth0 bearer** (operator JWT) | Debugger UI when `AUTH0_ENABLED=true` | The UI picks a project expl\"}, {\"path\": \"docs-site/concepts/cost-and-tokens.mdx\", \"score\": 37, \"snippet\": \"--- title: \\\"Cost & tokens\\\" description: \\\"How per-span token counts and cost values roll up into trace and session totals.\\\" --- Cost and token counts are first-class in Continua. The SDK lets you attach them to any span; the platform rolls them up to the trace level and the debugger surfaces them in multiple places. ## The data path 1. Inside a span, the SDK records counts via `set_tokens` and `set_cost`: ```python with span(\\\"call_model\\\", kind=\\\"llm\\\") as s: answer = call_model(prompt) s.set_tokens(prompt=120, completion=48) s.set_cost(0.0023) ``` 2. The ingest path persists these on the span row. 3. A rollup River worker recomputes per-trace totals (sum of cost, sum of token counts per bucket,\"}, {\"path\": \"docs-site/concepts/data-model.mdx\", \"score\": 37, \"snippet\": \"--- title: \\\"Data model\\\" description: \\\"Projects, sessions, traces, spans, events: the persisted shapes that back the debugger.\\\" --- ```mermaid erDiagram projects ||--o{ ingest_batches : \\\"owns\\\" projects ||--o{ sessions : \\\"owns\\\" projects ||--o{ traces : \\\"owns\\\" projects ||--o{ spans : \\\"owns\\\" projects ||--o{ span_events : \\\"owns\\\" sessions ||--o{ traces : \\\"groups\\\" traces ||--o{ spans : \\\"contains\\\" traces ||--o{ span_events : \\\"contains\\\" spans ||--o{ span_events : \\\"emits via span_id\\\" projects { uuid id PK string name string api_key_hash timestamptz created_at } ingest_batches { uuid id PK uuid project_id FK string batch_key string status int trace_count int span_count } sessions { uuid id PK uuid proj\"}, {\"path\": \"docs-site/concepts/events.mdx\", \"score\": 37, \"snippet\": \"--- title: \\\"Events\\\" description: \\\"Semantic event types that explain what happened inside a span: state changes, decisions, effects, waits, and more.\\\" --- Continua events are **debugger-facing observability signals**. They explain *what happened* during trace execution. They are not replay primitives, checkpoints, or durable workflow state machines. ```mermaid flowchart LR subgraph Operational log metric end subgraph Failure error exception end subgraph Semantic state_change decision effect wait end subgraph Milestone message snapshot_marker end subgraph Domain custom end ``` ## Choosing an event type - **`state_change`**: observable state transitions you want shown as `old \u2192 new` in the debu\"}]}",
            "role": "user"
          }
        ],
        "input_truncated": false,
        "kind": "LLM",
        "latency_ms": 5091,
        "model": "gpt-4.1-mini-2025-04-14",
        "name": "synthesize_results",
        "output": {
          "confidence": "Moderate confidence based on indirect evidence from project, event, and cost/token documentation describing observability constructs and their intended uses. No direct literature or case studies on AI observability benefits were retrieved, so synthesis relies on extrapolation from available technical descriptions.",
          "risks": "The retrieved documents do not explicitly discuss risks or limitations of AI observability. Potential risks inferred include complexity in instrumenting AI systems to generate meaningful observability data, possible overhead in performance and cost due to detailed tracing, and challenges in interpreting rich semantic events without adequate tooling or expertise.",
          "synthesis": "AI observability refers to the comprehensive monitoring and analysis of AI system behaviors through detailed traces, spans, events, and metrics that capture model execution and decision-making processes. Key components include projects (multi-tenancy units), spans (units of work), events (semantic signals explaining span behavior), and cost/token tracking for resource usage. The primary benefits of AI observability are improved debugging and root cause analysis by providing semantic insights into model operations, enhanced operational monitoring through detailed event types and metrics, and better resource management via cost and token accounting. Compared to traditional monitoring, AI observability offers domain-specific signals (e.g., state changes, decisions) that enable deeper understanding of AI workflows, leading to faster issue resolution, increased trust in AI outputs, and operational efficiency. These benefits span technical improvements (model performance and reliability), operational advantages (streamlined debugging and monitoring), and business impacts (cost control and compliance)."
        },
        "output_truncated": false,
        "provider": "openai",
        "span_id": "88a273b3-00ff-4bad-bf98-c6558775ce3a",
        "started_at": "2026-06-03T00:42:14.336133+05:30",
        "status": "COMPLETED",
        "tokens_in": 962,
        "tokens_out": 307,
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      }
    ]
  }
}));

const traceTimelinesByTrace = new Map(Object.entries({
  "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4": {
    "events": [
      {
        "event_type": "span_started",
        "id": "ddf7a09e-f915-4094-bef8-7747b87954f8:span_started",
        "source": "synthetic",
        "span_id": "ddf7a09e-f915-4094-bef8-7747b87954f8",
        "span_name": "initial_step",
        "timestamp": "2026-06-03T00:42:24.479532+05:30",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      },
      {
        "event_type": "effect",
        "id": "76e6933a-0100-44c3-86e8-c956f2006b22",
        "level": "info",
        "message": "Model call: gpt-4.1-mini-2025-04-14",
        "payload": {
          "effect_id": "effect_0b971df263d7c702e466721ff05beec6",
          "effect_kind": "model_call",
          "has_external_side_effect": false
        },
        "sequence": 1,
        "source": "explicit",
        "span_id": "ddf7a09e-f915-4094-bef8-7747b87954f8",
        "span_name": "initial_step",
        "timestamp": "2026-06-03T00:42:26.23462+05:30",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      },
      {
        "event_type": "decision",
        "id": "a2dafb03-ebe2-44f2-83c8-12a8ae0e2e52",
        "level": "info",
        "message": "Planned dependency probe",
        "payload": {
          "chosen": "Verify the status and connectivity of all external dependencies and services that the agent workflow relies on.",
          "question": "Which dependency check should run first?",
          "reasoning": "A stalled agent workflow often results from issues with external dependencies such as APIs, databases, or third-party services. Checking their status and connectivity first helps identify if the stall is due to an unavailable or unresponsive dependency, allowing for targeted troubleshooting."
        },
        "sequence": 2,
        "source": "explicit",
        "span_id": "ddf7a09e-f915-4094-bef8-7747b87954f8",
        "span_name": "initial_step",
        "timestamp": "2026-06-03T00:42:26.23468+05:30",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      },
      {
        "event_type": "span_completed",
        "id": "ddf7a09e-f915-4094-bef8-7747b87954f8:span_completed",
        "source": "synthetic",
        "span_id": "ddf7a09e-f915-4094-bef8-7747b87954f8",
        "span_name": "initial_step",
        "timestamp": "2026-06-03T00:42:26.234691+05:30",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      },
      {
        "event_type": "span_started",
        "id": "39b893e2-5960-4323-bb5a-e21009dc8ed0:span_started",
        "source": "synthetic",
        "span_id": "39b893e2-5960-4323-bb5a-e21009dc8ed0",
        "span_name": "dependency_health_probe",
        "timestamp": "2026-06-03T00:42:26.234781+05:30",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      },
      {
        "event_type": "effect",
        "id": "1331cfab-bc3b-4d64-8754-75e0d5fee8a6",
        "level": "info",
        "message": "Tool call: dependency_health_probe",
        "payload": {
          "effect_id": "effect_e2f97348a39d06c9174faa326e01c43b",
          "effect_kind": "tool_call",
          "has_external_side_effect": false
        },
        "sequence": 1,
        "source": "explicit",
        "span_id": "39b893e2-5960-4323-bb5a-e21009dc8ed0",
        "span_name": "dependency_health_probe",
        "timestamp": "2026-06-03T00:42:26.25014+05:30",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      },
      {
        "event_type": "wait",
        "id": "e94c647c-d6c9-4ec2-8a4f-9ddc2d5525c6",
        "level": "info",
        "message": "Resolved wait: dependency health",
        "payload": {
          "duration_ms": 13.86,
          "error": "[Errno 61] Connection refused",
          "error_type": "ConnectError",
          "phase": "resolved",
          "resolution": "failed",
          "status": "failed",
          "target": "http://127.0.0.1:9/continua-demo-health",
          "wait_id": "dependency-probe-public-demo",
          "wait_kind": "dependency_health"
        },
        "sequence": 2,
        "source": "explicit",
        "span_id": "39b893e2-5960-4323-bb5a-e21009dc8ed0",
        "span_name": "dependency_health_probe",
        "timestamp": "2026-06-03T00:42:26.250221+05:30",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      },
      {
        "event_type": "error",
        "id": "6d146282-abe5-411d-a55f-56747f645aae",
        "level": "error",
        "message": "Dependency health check failed",
        "payload": {
          "duration_ms": 13.86,
          "error": "[Errno 61] Connection refused",
          "error_type": "ConnectError",
          "status": "failed",
          "target": "http://127.0.0.1:9/continua-demo-health"
        },
        "sequence": 3,
        "source": "explicit",
        "span_id": "39b893e2-5960-4323-bb5a-e21009dc8ed0",
        "span_name": "dependency_health_probe",
        "timestamp": "2026-06-03T00:42:26.250237+05:30",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      },
      {
        "event_type": "exception",
        "id": "94ca0560-6a56-41f1-a7f8-d789bc046bf6",
        "level": "error",
        "message": "Dependency health check failed",
        "payload": {
          "duration_ms": 13.86,
          "error": "[Errno 61] Connection refused",
          "error_type": "ConnectError",
          "exception_message": "Dependency health check failed",
          "exception_type": "TimeoutError",
          "status": "failed",
          "target": "http://127.0.0.1:9/continua-demo-health",
          "traceback": "TimeoutError: Dependency health check failed\n"
        },
        "sequence": 4,
        "source": "explicit",
        "span_id": "39b893e2-5960-4323-bb5a-e21009dc8ed0",
        "span_name": "dependency_health_probe",
        "timestamp": "2026-06-03T00:42:26.250634+05:30",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      },
      {
        "event_type": "span_failed",
        "id": "39b893e2-5960-4323-bb5a-e21009dc8ed0:span_failed",
        "source": "synthetic",
        "span_id": "39b893e2-5960-4323-bb5a-e21009dc8ed0",
        "span_name": "dependency_health_probe",
        "timestamp": "2026-06-03T00:42:26.250674+05:30",
        "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
      }
    ],
    "has_more": false,
    "poll_cursor": "eyJjdHMiOiIyMDI2LTA2LTAyVDE5OjEyOjI2LjI1MzY4M1oiLCJzcmMiOiJzeW50aGV0aWMiLCJpZCI6IjM5Yjg5M2UyLTU5NjAtNDMyMy1iYjVhLWUyMTAwOWRjOGVkMDpzcGFuX2ZhaWxlZCJ9",
    "trace_status": "FAILED"
  },
  "88e6b423-cb06-47dc-a649-dd9737b47ee5": {
    "events": [
      {
        "event_type": "span_started",
        "id": "030fa1a6-efd1-4d28-bd15-f2005c0f7f02:span_started",
        "source": "synthetic",
        "span_id": "030fa1a6-efd1-4d28-bd15-f2005c0f7f02",
        "span_name": "code_analysis_chain",
        "timestamp": "2026-06-03T00:42:19.523347+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "span_started",
        "id": "b302775f-7769-44ff-9301-39c1ebef53c1:span_started",
        "source": "synthetic",
        "span_id": "b302775f-7769-44ff-9301-39c1ebef53c1",
        "span_name": "parse_python_source",
        "timestamp": "2026-06-03T00:42:19.523364+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "effect",
        "id": "c75ea429-24eb-4ad3-964e-98e308f362dd",
        "level": "info",
        "message": "Tool call: parse_python_source",
        "payload": {
          "effect_id": "effect_a869d546d1aa9f4fdfec9b80ec8cc637",
          "effect_kind": "tool_call",
          "has_external_side_effect": false
        },
        "sequence": 1,
        "source": "explicit",
        "span_id": "b302775f-7769-44ff-9301-39c1ebef53c1",
        "span_name": "parse_python_source",
        "timestamp": "2026-06-03T00:42:19.524029+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "span_completed",
        "id": "b302775f-7769-44ff-9301-39c1ebef53c1:span_completed",
        "source": "synthetic",
        "span_id": "b302775f-7769-44ff-9301-39c1ebef53c1",
        "span_name": "parse_python_source",
        "timestamp": "2026-06-03T00:42:19.524046+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "span_started",
        "id": "11854168-97f4-40d4-a2e8-de094f2a7d0e:span_started",
        "source": "synthetic",
        "span_id": "11854168-97f4-40d4-a2e8-de094f2a7d0e",
        "span_name": "local_quality_scan",
        "timestamp": "2026-06-03T00:42:19.524066+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "effect",
        "id": "cdfb0c42-cb34-488a-82de-b4c313f63be8",
        "level": "info",
        "message": "Tool call: local_quality_scan",
        "payload": {
          "effect_id": "effect_b01db6b21051d8020859e3a6c710a218",
          "effect_kind": "tool_call",
          "has_external_side_effect": false
        },
        "sequence": 1,
        "source": "explicit",
        "span_id": "11854168-97f4-40d4-a2e8-de094f2a7d0e",
        "span_name": "local_quality_scan",
        "timestamp": "2026-06-03T00:42:19.550553+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "span_completed",
        "id": "11854168-97f4-40d4-a2e8-de094f2a7d0e:span_completed",
        "source": "synthetic",
        "span_id": "11854168-97f4-40d4-a2e8-de094f2a7d0e",
        "span_name": "local_quality_scan",
        "timestamp": "2026-06-03T00:42:19.550576+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "span_completed",
        "id": "030fa1a6-efd1-4d28-bd15-f2005c0f7f02:span_completed",
        "source": "synthetic",
        "span_id": "030fa1a6-efd1-4d28-bd15-f2005c0f7f02",
        "span_name": "code_analysis_chain",
        "timestamp": "2026-06-03T00:42:19.550583+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "span_started",
        "id": "692bf577-2028-4630-b104-87e3eefbfb68:span_started",
        "source": "synthetic",
        "span_id": "692bf577-2028-4630-b104-87e3eefbfb68",
        "span_name": "generate_review",
        "timestamp": "2026-06-03T00:42:19.550604+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "effect",
        "id": "73d886a0-cd38-44ce-acf8-d33a87eec132",
        "level": "info",
        "message": "Model call: gpt-4.1-mini-2025-04-14",
        "payload": {
          "effect_id": "effect_4bfa1b5a263db7919feb0cd8d0a1a216",
          "effect_kind": "model_call",
          "has_external_side_effect": false
        },
        "sequence": 1,
        "source": "explicit",
        "span_id": "692bf577-2028-4630-b104-87e3eefbfb68",
        "span_name": "generate_review",
        "timestamp": "2026-06-03T00:42:24.449845+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "decision",
        "id": "ad17fa9e-d160-4168-8380-5ba86d4d6acd",
        "level": "info",
        "message": "Completed code review decision",
        "payload": {
          "alternatives": [
            "fail",
            "pass_with_notes",
            "pass"
          ],
          "chosen": "pass_with_notes",
          "question": "Should this module pass the demo review?",
          "reasoning": "The Python SDK tracing module provides a TraceContext class for managing trace lifecycles and a get_current_trace function to access the current trace context. It uses contextvars to maintain trace state across asynchronous calls. The TraceContext supports setting inputs, outputs, metadata, and automatically sends trace data to a server. The module is well-structured with 14 functions and no classes other than TraceContext. There are no long lines detected, and contextvars usage is appropriate. However, the trace start is not sent before user input, which may affect trace accuracy."
        },
        "sequence": 2,
        "source": "explicit",
        "span_id": "692bf577-2028-4630-b104-87e3eefbfb68",
        "span_name": "generate_review",
        "timestamp": "2026-06-03T00:42:24.450143+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      },
      {
        "event_type": "span_completed",
        "id": "692bf577-2028-4630-b104-87e3eefbfb68:span_completed",
        "source": "synthetic",
        "span_id": "692bf577-2028-4630-b104-87e3eefbfb68",
        "span_name": "generate_review",
        "timestamp": "2026-06-03T00:42:24.450246+05:30",
        "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
      }
    ],
    "has_more": false,
    "poll_cursor": "eyJjdHMiOiIyMDI2LTA2LTAyVDE5OjEyOjI1LjE3NTQ4NVoiLCJzcmMiOiJleHBsaWNpdCIsImlkIjoiYWQxN2ZhOWUtZDE2MC00MTY4LTgzODAtNWJhODZkNGQ2YWNkIn0",
    "trace_status": "COMPLETED"
  },
  "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7": {
    "events": [
      {
        "event_type": "span_started",
        "id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d:span_started",
        "source": "synthetic",
        "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
        "span_name": "plan_research",
        "timestamp": "2026-06-03T00:42:10.857923+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "log",
        "id": "6d562fcd-42b6-42bc-bd3c-74d3909bb384",
        "level": "info",
        "message": "Planning research steps",
        "payload": {
          "query": "What are the benefits of AI observability?"
        },
        "sequence": 1,
        "source": "explicit",
        "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
        "span_name": "plan_research",
        "timestamp": "2026-06-03T00:42:10.857941+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "effect",
        "id": "9fe1741f-8444-4d75-a8f7-af1a8e34cca9",
        "level": "info",
        "message": "Model call: gpt-4.1-mini-2025-04-14",
        "payload": {
          "effect_id": "effect_08f8e9780b5260e9d4c31d5e1631bf4b",
          "effect_kind": "model_call",
          "has_external_side_effect": false
        },
        "sequence": 2,
        "source": "explicit",
        "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
        "span_name": "plan_research",
        "timestamp": "2026-06-03T00:42:14.330431+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "decision",
        "id": "812864cd-9694-4f92-822e-3fb4a66846fa",
        "level": "info",
        "message": "Selected research focus",
        "payload": {
          "alternatives": [
            "Focus on technical benefits of AI observability such as improved model performance and debugging.",
            "Explore business impacts like increased trust and compliance due to AI observability.",
            "Investigate challenges and limitations alongside benefits to provide a balanced view."
          ],
          "chosen": "Identify and analyze the primary benefits of AI observability through literature review and case studies.",
          "question": "Which research focus should the agent pursue?",
          "reasoning": "['Define AI observability and its key components.', 'Identify the primary benefits of AI observability from academic papers, industry reports, and expert articles.', 'Analyze case studies where AI observability has been implemented to understand practical advantages.', 'Compare AI observability benefits with traditional monitoring approaches.', 'Summarize findings highlighting technical, operational, and business benefits.']"
        },
        "sequence": 3,
        "source": "explicit",
        "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
        "span_name": "plan_research",
        "timestamp": "2026-06-03T00:42:14.330482+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "span_completed",
        "id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d:span_completed",
        "source": "synthetic",
        "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
        "span_name": "plan_research",
        "timestamp": "2026-06-03T00:42:14.330518+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "span_started",
        "id": "5bfb0245-5353-4c22-9e75-55b3185a700f:span_started",
        "source": "synthetic",
        "span_id": "5bfb0245-5353-4c22-9e75-55b3185a700f",
        "span_name": "docs_search",
        "timestamp": "2026-06-03T00:42:14.330584+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "effect",
        "id": "a705fe93-1b61-41c2-9861-d59489c4897b",
        "level": "info",
        "message": "Tool call: docs_search",
        "payload": {
          "effect_id": "effect_507707d803fb5fea44b9e7644c5d2c06",
          "effect_kind": "tool_call",
          "has_external_side_effect": false
        },
        "sequence": 1,
        "source": "explicit",
        "span_id": "5bfb0245-5353-4c22-9e75-55b3185a700f",
        "span_name": "docs_search",
        "timestamp": "2026-06-03T00:42:14.336036+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "metric",
        "id": "80636386-69da-4c45-8566-9389c14b0490",
        "level": "info",
        "payload": {
          "metric_name": "match_count",
          "metric_value": 4,
          "stage": "retrieval"
        },
        "sequence": 2,
        "source": "explicit",
        "span_id": "5bfb0245-5353-4c22-9e75-55b3185a700f",
        "span_name": "docs_search",
        "timestamp": "2026-06-03T00:42:14.336063+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "span_completed",
        "id": "5bfb0245-5353-4c22-9e75-55b3185a700f:span_completed",
        "source": "synthetic",
        "span_id": "5bfb0245-5353-4c22-9e75-55b3185a700f",
        "span_name": "docs_search",
        "timestamp": "2026-06-03T00:42:14.336087+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "span_started",
        "id": "88a273b3-00ff-4bad-bf98-c6558775ce3a:span_started",
        "source": "synthetic",
        "span_id": "88a273b3-00ff-4bad-bf98-c6558775ce3a",
        "span_name": "synthesize_results",
        "timestamp": "2026-06-03T00:42:14.336133+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "effect",
        "id": "368e6b39-df0a-4058-94f4-26d342782d51",
        "level": "info",
        "message": "Model call: gpt-4.1-mini-2025-04-14",
        "payload": {
          "effect_id": "effect_a5edd12cb2710b497d0e128ceca781f0",
          "effect_kind": "model_call",
          "has_external_side_effect": false
        },
        "sequence": 1,
        "source": "explicit",
        "span_id": "88a273b3-00ff-4bad-bf98-c6558775ce3a",
        "span_name": "synthesize_results",
        "timestamp": "2026-06-03T00:42:19.426947+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "snapshot_marker",
        "id": "2fb2fe9e-98f1-4d43-b422-260093237915",
        "level": "info",
        "message": "Research synthesis complete",
        "payload": {
          "label": "Research synthesis complete",
          "marker_kind": "agent_step",
          "source_count": 4
        },
        "sequence": 2,
        "source": "explicit",
        "span_id": "88a273b3-00ff-4bad-bf98-c6558775ce3a",
        "span_name": "synthesize_results",
        "timestamp": "2026-06-03T00:42:19.427014+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      },
      {
        "event_type": "span_completed",
        "id": "88a273b3-00ff-4bad-bf98-c6558775ce3a:span_completed",
        "source": "synthetic",
        "span_id": "88a273b3-00ff-4bad-bf98-c6558775ce3a",
        "span_name": "synthesize_results",
        "timestamp": "2026-06-03T00:42:19.427057+05:30",
        "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
      }
    ],
    "has_more": false,
    "poll_cursor": "eyJjdHMiOiIyMDI2LTA2LTAyVDE5OjEyOjE5LjUyNTAyWiIsInNyYyI6ImV4cGxpY2l0IiwiaWQiOiIyZmIyZmU5ZS05OGYxLTRkNDMtYjQyMi0yNjAwOTMyMzc5MTUifQ",
    "trace_status": "COMPLETED"
  }
}));

const emptySpans = { spans: [] };
const emptyTimeline = { events: [], trace_status: 'RUNNING', has_more: false };

const sessionNarrativesBySession = new Map(Object.entries({
  "48cffb69-9c27-45f9-9db3-1a0f5b66f50f": {
    "summary": {
      "completed_trace_count": 0,
      "explicit_link_count": 0,
      "failed_trace_count": 1,
      "inferred_link_count": 0,
      "last_activity_at": "2026-06-03T00:42:26.250695+05:30",
      "returned_trace_count": 1,
      "running_trace_count": 0,
      "started_at": "2026-06-03T00:42:24.479453+05:30",
      "total_cost_usd": 0,
      "total_tokens_in": 38,
      "total_tokens_out": 80,
      "total_trace_count": 1,
      "truncated": false,
      "unlinked_trace_count": 1
    },
    "traces": [
      {
        "duration_ms": 1771,
        "ended_at": "2026-06-03T00:42:26.250695+05:30",
        "error_count": 1,
        "id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4",
        "latest_activity_at": "2026-06-03T00:42:26.253683+05:30",
        "lineage": {
          "type": "unlinked"
        },
        "name": "failing_agent",
        "semantic_events": [
          {
            "event_type": "effect",
            "id": "76e6933a-0100-44c3-86e8-c956f2006b22",
            "level": "info",
            "message": "Model call: gpt-4.1-mini-2025-04-14",
            "payload": {
              "effect_id": "effect_0b971df263d7c702e466721ff05beec6",
              "effect_kind": "model_call",
              "has_external_side_effect": false
            },
            "sequence": 1,
            "source": "explicit",
            "span_id": "ddf7a09e-f915-4094-bef8-7747b87954f8",
            "span_name": "initial_step",
            "timestamp": "2026-06-03T00:42:26.23462+05:30",
            "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
          },
          {
            "event_type": "decision",
            "id": "a2dafb03-ebe2-44f2-83c8-12a8ae0e2e52",
            "level": "info",
            "message": "Planned dependency probe",
            "payload": {
              "chosen": "Verify the status and connectivity of all external dependencies and services that the agent workflow relies on.",
              "question": "Which dependency check should run first?",
              "reasoning": "A stalled agent workflow often results from issues with external dependencies such as APIs, databases, or third-party services. Checking their status and connectivity first helps identify if the stall is due to an unavailable or unresponsive dependency, allowing for targeted troubleshooting."
            },
            "sequence": 2,
            "source": "explicit",
            "span_id": "ddf7a09e-f915-4094-bef8-7747b87954f8",
            "span_name": "initial_step",
            "timestamp": "2026-06-03T00:42:26.23468+05:30",
            "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
          },
          {
            "event_type": "effect",
            "id": "1331cfab-bc3b-4d64-8754-75e0d5fee8a6",
            "level": "info",
            "message": "Tool call: dependency_health_probe",
            "payload": {
              "effect_id": "effect_e2f97348a39d06c9174faa326e01c43b",
              "effect_kind": "tool_call",
              "has_external_side_effect": false
            },
            "sequence": 1,
            "source": "explicit",
            "span_id": "39b893e2-5960-4323-bb5a-e21009dc8ed0",
            "span_name": "dependency_health_probe",
            "timestamp": "2026-06-03T00:42:26.25014+05:30",
            "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
          },
          {
            "event_type": "wait",
            "id": "e94c647c-d6c9-4ec2-8a4f-9ddc2d5525c6",
            "level": "info",
            "message": "Resolved wait: dependency health",
            "payload": {
              "duration_ms": 13.86,
              "error": "[Errno 61] Connection refused",
              "error_type": "ConnectError",
              "phase": "resolved",
              "resolution": "failed",
              "status": "failed",
              "target": "http://127.0.0.1:9/continua-demo-health",
              "wait_id": "dependency-probe-public-demo",
              "wait_kind": "dependency_health"
            },
            "sequence": 2,
            "source": "explicit",
            "span_id": "39b893e2-5960-4323-bb5a-e21009dc8ed0",
            "span_name": "dependency_health_probe",
            "timestamp": "2026-06-03T00:42:26.250221+05:30",
            "trace_id": "e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4"
          }
        ],
        "started_at": "2026-06-03T00:42:24.479453+05:30",
        "status": "FAILED",
        "total_cost_usd": 0,
        "total_tokens_in": 38,
        "total_tokens_out": 80,
        "trace_id": "13f51fd2-053a-48af-ac50-1ae97bc56087"
      }
    ]
  },
  "cfb33ecf-2148-489f-84e7-a9f366dc2377": {
    "summary": {
      "completed_trace_count": 2,
      "explicit_link_count": 0,
      "failed_trace_count": 0,
      "inferred_link_count": 0,
      "last_activity_at": "2026-06-03T00:42:24.45114+05:30",
      "returned_trace_count": 2,
      "running_trace_count": 0,
      "started_at": "2026-06-03T00:42:10.857567+05:30",
      "total_cost_usd": 0,
      "total_tokens_in": 1613,
      "total_tokens_out": 801,
      "total_trace_count": 2,
      "truncated": false,
      "unlinked_trace_count": 2
    },
    "traces": [
      {
        "duration_ms": 8570,
        "ended_at": "2026-06-03T00:42:19.427109+05:30",
        "error_count": 0,
        "id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7",
        "latest_activity_at": "2026-06-03T00:42:19.52502+05:30",
        "lineage": {
          "type": "unlinked"
        },
        "name": "research_agent",
        "semantic_events": [
          {
            "event_type": "effect",
            "id": "9fe1741f-8444-4d75-a8f7-af1a8e34cca9",
            "level": "info",
            "message": "Model call: gpt-4.1-mini-2025-04-14",
            "payload": {
              "effect_id": "effect_08f8e9780b5260e9d4c31d5e1631bf4b",
              "effect_kind": "model_call",
              "has_external_side_effect": false
            },
            "sequence": 2,
            "source": "explicit",
            "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
            "span_name": "plan_research",
            "timestamp": "2026-06-03T00:42:14.330431+05:30",
            "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
          },
          {
            "event_type": "decision",
            "id": "812864cd-9694-4f92-822e-3fb4a66846fa",
            "level": "info",
            "message": "Selected research focus",
            "payload": {
              "alternatives": [
                "Focus on technical benefits of AI observability such as improved model performance and debugging.",
                "Explore business impacts like increased trust and compliance due to AI observability.",
                "Investigate challenges and limitations alongside benefits to provide a balanced view."
              ],
              "chosen": "Identify and analyze the primary benefits of AI observability through literature review and case studies.",
              "question": "Which research focus should the agent pursue?",
              "reasoning": "['Define AI observability and its key components.', 'Identify the primary benefits of AI observability from academic papers, industry reports, and expert articles.', 'Analyze case studies where AI observability has been implemented to understand practical advantages.', 'Compare AI observability benefits with traditional monitoring approaches.', 'Summarize findings highlighting technical, operational, and business benefits.']"
            },
            "sequence": 3,
            "source": "explicit",
            "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
            "span_name": "plan_research",
            "timestamp": "2026-06-03T00:42:14.330482+05:30",
            "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
          },
          {
            "event_type": "effect",
            "id": "a705fe93-1b61-41c2-9861-d59489c4897b",
            "level": "info",
            "message": "Tool call: docs_search",
            "payload": {
              "effect_id": "effect_507707d803fb5fea44b9e7644c5d2c06",
              "effect_kind": "tool_call",
              "has_external_side_effect": false
            },
            "sequence": 1,
            "source": "explicit",
            "span_id": "5bfb0245-5353-4c22-9e75-55b3185a700f",
            "span_name": "docs_search",
            "timestamp": "2026-06-03T00:42:14.336036+05:30",
            "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
          },
          {
            "event_type": "effect",
            "id": "368e6b39-df0a-4058-94f4-26d342782d51",
            "level": "info",
            "message": "Model call: gpt-4.1-mini-2025-04-14",
            "payload": {
              "effect_id": "effect_a5edd12cb2710b497d0e128ceca781f0",
              "effect_kind": "model_call",
              "has_external_side_effect": false
            },
            "sequence": 1,
            "source": "explicit",
            "span_id": "88a273b3-00ff-4bad-bf98-c6558775ce3a",
            "span_name": "synthesize_results",
            "timestamp": "2026-06-03T00:42:19.426947+05:30",
            "trace_id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7"
          }
        ],
        "started_at": "2026-06-03T00:42:10.857567+05:30",
        "status": "COMPLETED",
        "total_cost_usd": 0,
        "total_tokens_in": 1009,
        "total_tokens_out": 472,
        "trace_id": "26eb1cf0-3191-45c0-939b-776a1cb8d85e"
      },
      {
        "duration_ms": 4928,
        "ended_at": "2026-06-03T00:42:24.45114+05:30",
        "error_count": 0,
        "id": "88e6b423-cb06-47dc-a649-dd9737b47ee5",
        "latest_activity_at": "2026-06-03T00:42:25.175485+05:30",
        "lineage": {
          "type": "unlinked"
        },
        "name": "code_review_agent",
        "semantic_events": [
          {
            "event_type": "effect",
            "id": "c75ea429-24eb-4ad3-964e-98e308f362dd",
            "level": "info",
            "message": "Tool call: parse_python_source",
            "payload": {
              "effect_id": "effect_a869d546d1aa9f4fdfec9b80ec8cc637",
              "effect_kind": "tool_call",
              "has_external_side_effect": false
            },
            "sequence": 1,
            "source": "explicit",
            "span_id": "b302775f-7769-44ff-9301-39c1ebef53c1",
            "span_name": "parse_python_source",
            "timestamp": "2026-06-03T00:42:19.524029+05:30",
            "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
          },
          {
            "event_type": "effect",
            "id": "cdfb0c42-cb34-488a-82de-b4c313f63be8",
            "level": "info",
            "message": "Tool call: local_quality_scan",
            "payload": {
              "effect_id": "effect_b01db6b21051d8020859e3a6c710a218",
              "effect_kind": "tool_call",
              "has_external_side_effect": false
            },
            "sequence": 1,
            "source": "explicit",
            "span_id": "11854168-97f4-40d4-a2e8-de094f2a7d0e",
            "span_name": "local_quality_scan",
            "timestamp": "2026-06-03T00:42:19.550553+05:30",
            "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
          },
          {
            "event_type": "effect",
            "id": "73d886a0-cd38-44ce-acf8-d33a87eec132",
            "level": "info",
            "message": "Model call: gpt-4.1-mini-2025-04-14",
            "payload": {
              "effect_id": "effect_4bfa1b5a263db7919feb0cd8d0a1a216",
              "effect_kind": "model_call",
              "has_external_side_effect": false
            },
            "sequence": 1,
            "source": "explicit",
            "span_id": "692bf577-2028-4630-b104-87e3eefbfb68",
            "span_name": "generate_review",
            "timestamp": "2026-06-03T00:42:24.449845+05:30",
            "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
          },
          {
            "event_type": "decision",
            "id": "ad17fa9e-d160-4168-8380-5ba86d4d6acd",
            "level": "info",
            "message": "Completed code review decision",
            "payload": {
              "alternatives": [
                "fail",
                "pass_with_notes",
                "pass"
              ],
              "chosen": "pass_with_notes",
              "question": "Should this module pass the demo review?",
              "reasoning": "The Python SDK tracing module provides a TraceContext class for managing trace lifecycles and a get_current_trace function to access the current trace context. It uses contextvars to maintain trace state across asynchronous calls. The TraceContext supports setting inputs, outputs, metadata, and automatically sends trace data to a server. The module is well-structured with 14 functions and no classes other than TraceContext. There are no long lines detected, and contextvars usage is appropriate. However, the trace start is not sent before user input, which may affect trace accuracy."
            },
            "sequence": 2,
            "source": "explicit",
            "span_id": "692bf577-2028-4630-b104-87e3eefbfb68",
            "span_name": "generate_review",
            "timestamp": "2026-06-03T00:42:24.450143+05:30",
            "trace_id": "88e6b423-cb06-47dc-a649-dd9737b47ee5"
          }
        ],
        "started_at": "2026-06-03T00:42:19.523311+05:30",
        "status": "COMPLETED",
        "total_cost_usd": 0,
        "total_tokens_in": 604,
        "total_tokens_out": 329,
        "trace_id": "ac89ef24-ac9f-4f9a-87d5-15922848e10b"
      }
    ]
  }
}));

const sessionComparesBySession = new Map(Object.entries({
  "cfb33ecf-2148-489f-84e7-a9f366dc2377": {
    "session": {
      "external_id": "demo-sdk-public-demo-research",
      "id": "cfb33ecf-2148-489f-84e7-a9f366dc2377",
      "name": "Real agent demo: demo-sdk-public-demo-research"
    },
    "baseline": {
      "duration_ms": 8570,
      "ended_at": "2026-06-03T00:42:19.427109+05:30",
      "error_count": 0,
      "id": "1566b1e5-22bb-4484-8d5e-de0b59dbfbb7",
      "name": "research_agent",
      "started_at": "2026-06-03T00:42:10.857567+05:30",
      "status": "COMPLETED",
      "total_cost_usd": 0,
      "total_tokens_in": 1009,
      "total_tokens_out": 472,
      "trace_id": "26eb1cf0-3191-45c0-939b-776a1cb8d85e"
    },
    "candidate": {
      "duration_ms": 4928,
      "ended_at": "2026-06-03T00:42:24.45114+05:30",
      "error_count": 0,
      "id": "88e6b423-cb06-47dc-a649-dd9737b47ee5",
      "name": "code_review_agent",
      "started_at": "2026-06-03T00:42:19.523311+05:30",
      "status": "COMPLETED",
      "total_cost_usd": 0,
      "total_tokens_in": 604,
      "total_tokens_out": 329,
      "trace_id": "ac89ef24-ac9f-4f9a-87d5-15922848e10b"
    },
    "summary": {
      "cost_delta_usd": 0,
      "duration_delta_ms": -3642,
      "heuristic_matches": 0,
      "matched_spans": 0,
      "tokens_in_delta": -405,
      "tokens_out_delta": -143,
      "total_semantic_baseline": 4,
      "total_semantic_candidate": 4,
      "total_spans_baseline": 3,
      "total_spans_candidate": 4,
      "unmatched_baseline_spans": 3,
      "unmatched_candidate_spans": 4
    },
    "span_diffs": [
      {
        "diff_status": "baseline_only",
        "match_source": null,
        "match_reason": null,
        "changed_fields": [],
        "baseline_span": {
          "ended_at": "2026-06-03T00:42:14.330518+05:30",
          "id": "ac99f487-c389-41ac-bdce-26c64e959b50",
          "kind": "LLM",
          "latency_ms": 3473,
          "model": "gpt-4.1-mini-2025-04-14",
          "name": "plan_research",
          "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
          "started_at": "2026-06-03T00:42:10.857923+05:30",
          "status": "COMPLETED",
          "tokens_in": 47,
          "tokens_out": 165
        },
        "candidate_span": null,
        "semantic_groups": [
          {
            "event_type": "effect",
            "diff_status": "baseline_only",
            "match_source": null,
            "match_reason": null,
            "changed_fields": [],
            "baseline_event": {
              "event_type": "effect",
              "id": "9fe1741f-8444-4d75-a8f7-af1a8e34cca9",
              "message": "Model call: gpt-4.1-mini-2025-04-14",
              "payload": {
                "effect_id": "effect_08f8e9780b5260e9d4c31d5e1631bf4b",
                "effect_kind": "model_call",
                "has_external_side_effect": false
              },
              "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
              "span_name": "plan_research",
              "timestamp": "2026-06-03T00:42:14.330431+05:30"
            },
            "candidate_event": null
          },
          {
            "event_type": "decision",
            "diff_status": "baseline_only",
            "match_source": null,
            "match_reason": null,
            "changed_fields": [],
            "baseline_event": {
              "event_type": "decision",
              "id": "812864cd-9694-4f92-822e-3fb4a66846fa",
              "message": "Selected research focus",
              "payload": {
                "alternatives": [
                  "Focus on technical benefits of AI observability such as improved model performance and debugging.",
                  "Explore business impacts like increased trust and compliance due to AI observability.",
                  "Investigate challenges and limitations alongside benefits to provide a balanced view."
                ],
                "chosen": "Identify and analyze the primary benefits of AI observability through literature review and case studies.",
                "question": "Which research focus should the agent pursue?",
                "reasoning": "['Define AI observability and its key components.', 'Identify the primary benefits of AI observability from academic papers, industry reports, and expert articles.', 'Analyze case studies where AI observability has been implemented to understand practical advantages.', 'Compare AI observability benefits with traditional monitoring approaches.', 'Summarize findings highlighting technical, operational, and business benefits.']"
              },
              "span_id": "d23d4288-ce24-4f71-b83b-c2d52e1c751d",
              "span_name": "plan_research",
              "timestamp": "2026-06-03T00:42:14.330482+05:30"
            },
            "candidate_event": null
          }
        ],
        "depth": 0
      },
      {
        "diff_status": "baseline_only",
        "match_source": null,
        "match_reason": null,
        "changed_fields": [],
        "baseline_span": {
          "ended_at": "2026-06-03T00:42:14.336087+05:30",
          "id": "0a50014a-bf7a-4249-83da-18bc5372a511",
          "kind": "TOOL",
          "latency_ms": 6,
          "name": "docs_search",
          "span_id": "5bfb0245-5353-4c22-9e75-55b3185a700f",
          "started_at": "2026-06-03T00:42:14.330584+05:30",
          "status": "COMPLETED"
        },
        "candidate_span": null,
        "semantic_groups": [
          {
            "event_type": "effect",
            "diff_status": "baseline_only",
            "match_source": null,
            "match_reason": null,
            "changed_fields": [],
            "baseline_event": {
              "event_type": "effect",
              "id": "a705fe93-1b61-41c2-9861-d59489c4897b",
              "message": "Tool call: docs_search",
              "payload": {
                "effect_id": "effect_507707d803fb5fea44b9e7644c5d2c06",
                "effect_kind": "tool_call",
                "has_external_side_effect": false
              },
              "span_id": "5bfb0245-5353-4c22-9e75-55b3185a700f",
              "span_name": "docs_search",
              "timestamp": "2026-06-03T00:42:14.336036+05:30"
            },
            "candidate_event": null
          }
        ],
        "depth": 0
      },
      {
        "diff_status": "baseline_only",
        "match_source": null,
        "match_reason": null,
        "changed_fields": [],
        "baseline_span": {
          "ended_at": "2026-06-03T00:42:19.427057+05:30",
          "id": "88fb2aa9-c259-401a-ad9c-1b70f6279211",
          "kind": "LLM",
          "latency_ms": 5091,
          "model": "gpt-4.1-mini-2025-04-14",
          "name": "synthesize_results",
          "span_id": "88a273b3-00ff-4bad-bf98-c6558775ce3a",
          "started_at": "2026-06-03T00:42:14.336133+05:30",
          "status": "COMPLETED",
          "tokens_in": 962,
          "tokens_out": 307
        },
        "candidate_span": null,
        "semantic_groups": [
          {
            "event_type": "effect",
            "diff_status": "baseline_only",
            "match_source": null,
            "match_reason": null,
            "changed_fields": [],
            "baseline_event": {
              "event_type": "effect",
              "id": "368e6b39-df0a-4058-94f4-26d342782d51",
              "message": "Model call: gpt-4.1-mini-2025-04-14",
              "payload": {
                "effect_id": "effect_a5edd12cb2710b497d0e128ceca781f0",
                "effect_kind": "model_call",
                "has_external_side_effect": false
              },
              "span_id": "88a273b3-00ff-4bad-bf98-c6558775ce3a",
              "span_name": "synthesize_results",
              "timestamp": "2026-06-03T00:42:19.426947+05:30"
            },
            "candidate_event": null
          }
        ],
        "depth": 0
      },
      {
        "diff_status": "candidate_only",
        "match_source": null,
        "match_reason": null,
        "changed_fields": [],
        "baseline_span": null,
        "candidate_span": {
          "ended_at": "2026-06-03T00:42:19.550583+05:30",
          "id": "ad6a8500-16d5-490b-9ae9-370e335b2ac0",
          "kind": "CHAIN",
          "latency_ms": 27,
          "name": "code_analysis_chain",
          "span_id": "030fa1a6-efd1-4d28-bd15-f2005c0f7f02",
          "started_at": "2026-06-03T00:42:19.523347+05:30",
          "status": "COMPLETED"
        },
        "semantic_groups": [],
        "depth": 0
      },
      {
        "diff_status": "candidate_only",
        "match_source": null,
        "match_reason": null,
        "changed_fields": [],
        "baseline_span": null,
        "candidate_span": {
          "ended_at": "2026-06-03T00:42:19.524046+05:30",
          "id": "ff7f27ae-e3cf-4bbb-af8c-fb3373e76124",
          "kind": "TOOL",
          "latency_ms": 1,
          "name": "parse_python_source",
          "parent_span_id": "030fa1a6-efd1-4d28-bd15-f2005c0f7f02",
          "span_id": "b302775f-7769-44ff-9301-39c1ebef53c1",
          "started_at": "2026-06-03T00:42:19.523364+05:30",
          "status": "COMPLETED"
        },
        "semantic_groups": [
          {
            "event_type": "effect",
            "diff_status": "candidate_only",
            "match_source": null,
            "match_reason": null,
            "changed_fields": [],
            "baseline_event": null,
            "candidate_event": {
              "event_type": "effect",
              "id": "c75ea429-24eb-4ad3-964e-98e308f362dd",
              "message": "Tool call: parse_python_source",
              "payload": {
                "effect_id": "effect_a869d546d1aa9f4fdfec9b80ec8cc637",
                "effect_kind": "tool_call",
                "has_external_side_effect": false
              },
              "span_id": "b302775f-7769-44ff-9301-39c1ebef53c1",
              "span_name": "parse_python_source",
              "timestamp": "2026-06-03T00:42:19.524029+05:30"
            }
          }
        ],
        "depth": 1
      },
      {
        "diff_status": "candidate_only",
        "match_source": null,
        "match_reason": null,
        "changed_fields": [],
        "baseline_span": null,
        "candidate_span": {
          "ended_at": "2026-06-03T00:42:19.550576+05:30",
          "id": "19f6dfb2-eeee-4a4c-a19d-e76c5b30646a",
          "kind": "TOOL",
          "latency_ms": 27,
          "name": "local_quality_scan",
          "parent_span_id": "030fa1a6-efd1-4d28-bd15-f2005c0f7f02",
          "span_id": "11854168-97f4-40d4-a2e8-de094f2a7d0e",
          "started_at": "2026-06-03T00:42:19.524066+05:30",
          "status": "COMPLETED"
        },
        "semantic_groups": [
          {
            "event_type": "effect",
            "diff_status": "candidate_only",
            "match_source": null,
            "match_reason": null,
            "changed_fields": [],
            "baseline_event": null,
            "candidate_event": {
              "event_type": "effect",
              "id": "cdfb0c42-cb34-488a-82de-b4c313f63be8",
              "message": "Tool call: local_quality_scan",
              "payload": {
                "effect_id": "effect_b01db6b21051d8020859e3a6c710a218",
                "effect_kind": "tool_call",
                "has_external_side_effect": false
              },
              "span_id": "11854168-97f4-40d4-a2e8-de094f2a7d0e",
              "span_name": "local_quality_scan",
              "timestamp": "2026-06-03T00:42:19.550553+05:30"
            }
          }
        ],
        "depth": 1
      },
      {
        "diff_status": "candidate_only",
        "match_source": null,
        "match_reason": null,
        "changed_fields": [],
        "baseline_span": null,
        "candidate_span": {
          "ended_at": "2026-06-03T00:42:24.450246+05:30",
          "id": "38eb3a04-7ce7-4f3b-90e9-b859e023e24f",
          "kind": "LLM",
          "latency_ms": 4900,
          "model": "gpt-4.1-mini-2025-04-14",
          "name": "generate_review",
          "span_id": "692bf577-2028-4630-b104-87e3eefbfb68",
          "started_at": "2026-06-03T00:42:19.550604+05:30",
          "status": "COMPLETED",
          "tokens_in": 604,
          "tokens_out": 329
        },
        "semantic_groups": [
          {
            "event_type": "effect",
            "diff_status": "candidate_only",
            "match_source": null,
            "match_reason": null,
            "changed_fields": [],
            "baseline_event": null,
            "candidate_event": {
              "event_type": "effect",
              "id": "73d886a0-cd38-44ce-acf8-d33a87eec132",
              "message": "Model call: gpt-4.1-mini-2025-04-14",
              "payload": {
                "effect_id": "effect_4bfa1b5a263db7919feb0cd8d0a1a216",
                "effect_kind": "model_call",
                "has_external_side_effect": false
              },
              "span_id": "692bf577-2028-4630-b104-87e3eefbfb68",
              "span_name": "generate_review",
              "timestamp": "2026-06-03T00:42:24.449845+05:30"
            }
          },
          {
            "event_type": "decision",
            "diff_status": "candidate_only",
            "match_source": null,
            "match_reason": null,
            "changed_fields": [],
            "baseline_event": null,
            "candidate_event": {
              "event_type": "decision",
              "id": "ad17fa9e-d160-4168-8380-5ba86d4d6acd",
              "message": "Completed code review decision",
              "payload": {
                "alternatives": [
                  "fail",
                  "pass_with_notes",
                  "pass"
                ],
                "chosen": "pass_with_notes",
                "question": "Should this module pass the demo review?",
                "reasoning": "The Python SDK tracing module provides a TraceContext class for managing trace lifecycles and a get_current_trace function to access the current trace context. It uses contextvars to maintain trace state across asynchronous calls. The TraceContext supports setting inputs, outputs, metadata, and automatically sends trace data to a server. The module is well-structured with 14 functions and no classes other than TraceContext. There are no long lines detected, and contextvars usage is appropriate. However, the trace start is not sent before user input, which may affect trace accuracy."
              },
              "span_id": "692bf577-2028-4630-b104-87e3eefbfb68",
              "span_name": "generate_review",
              "timestamp": "2026-06-03T00:42:24.450143+05:30"
            }
          }
        ],
        "depth": 0
      }
    ]
  }
}));

function json(data, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      'Content-Type': 'application/json; charset=utf-8',
      'Cache-Control': 'public, max-age=30',
    },
  });
}

function notFound() {
  return json({ code: 'not_found', message: 'Resource not found' }, 404);
}

function badRequest(message) {
  return json({ code: 'invalid_request', message }, 400);
}

function filterTraces(url) {
  let filtered = [...traces];
  const session = url.searchParams.get('session_id');
  const status = url.searchParams.get('status');
  const q = url.searchParams.get('q')?.toLowerCase();
  const userId = url.searchParams.get('user_id');
  const hasErrors = url.searchParams.get('has_errors') === 'true';
  const engineOnly = url.searchParams.get('engine_only') === 'true';

  if (engineOnly) {
    filtered = filtered.filter((trace) => trace.engine || trace.engine_run_id);
  }
  if (session) {
    filtered = filtered.filter((trace) => trace.session_id === session);
  }
  if (status) {
    filtered = filtered.filter((trace) => trace.status.toLowerCase() === status);
  }
  if (q) {
    filtered = filtered.filter((trace) => {
      const detail = traceDetails.get(trace.id);
      return (
        trace.name.toLowerCase().includes(q) ||
        detail?.user_id?.toLowerCase().includes(q) ||
        trace.session_external_id?.toLowerCase().includes(q)
      );
    });
  }
  if (userId) {
    filtered = filtered.filter((trace) => traceDetails.get(trace.id)?.user_id === userId);
  }
  if (hasErrors) {
    filtered = filtered.filter((trace) => (trace.error_count ?? 0) > 0);
  }

  const offset = Number(url.searchParams.get('offset') ?? '0');
  const limit = Number(url.searchParams.get('limit') ?? '20');
  return {
    total: filtered.length,
    traces: filtered.slice(offset, offset + limit),
  };
}

function filterSessions(url) {
  let filtered = [...sessions];
  const q = url.searchParams.get('q')?.toLowerCase();
  const userId = url.searchParams.get('user_id');

  if (q) {
    filtered = filtered.filter(
      (session) =>
        session.id.toLowerCase().includes(q) ||
        session.external_id.toLowerCase().includes(q) ||
        session.name?.toLowerCase().includes(q) ||
        session.user_id?.toLowerCase().includes(q)
    );
  }
  if (userId) {
    filtered = filtered.filter((session) => session.user_id === userId);
  }

  const offset = Number(url.searchParams.get('offset') ?? '0');
  const limit = Number(url.searchParams.get('limit') ?? '20');
  return {
    total: filtered.length,
    sessions: filtered.slice(offset, offset + limit),
  };
}

function handleApi(url) {
  if (url.pathname === '/api/auth/config') {
    return json({
      enabled: false,
      public_demo_enabled: true,
      public_demo_label: 'Real OpenAI-backed demo',
    });
  }

  if (url.pathname === '/api/projects') {
    return notFound();
  }

  if (url.pathname === '/api/traces') {
    return json(filterTraces(url));
  }

  if (/^\/api\/traces\/[^/]+\/spans$/.test(url.pathname)) {
    const traceId = url.pathname.split('/').at(-2);
    if (!traceDetails.has(traceId)) {
      return notFound();
    }
    return json(traceSpansByTrace.get(traceId) ?? emptySpans);
  }

  if (/^\/api\/traces\/[^/]+\/events$/.test(url.pathname)) {
    const traceId = url.pathname.split('/').at(-2);
    const detail = traceDetails.get(traceId);
    if (!detail) {
      return notFound();
    }
    const timeline =
      traceTimelinesByTrace.get(traceId) ??
      { ...emptyTimeline, trace_status: detail.status };
    return json(timeline);
  }

  if (/^\/api\/traces\/[^/]+$/.test(url.pathname)) {
    const traceId = url.pathname.split('/').at(-1);
    const detail = traceDetails.get(traceId);
    if (!detail) {
      return notFound();
    }
    return json(detail);
  }

  if (url.pathname === '/api/sessions') {
    return json(filterSessions(url));
  }

  if (/^\/api\/sessions\/[^/]+\/narrative$/.test(url.pathname)) {
    const id = url.pathname.split('/').at(-2);
    const narrative = sessionNarrativesBySession.get(id);
    return narrative ? json(narrative) : notFound();
  }

  if (/^\/api\/sessions\/[^/]+\/compare$/.test(url.pathname)) {
    const id = url.pathname.split('/').at(-2);
    const compare = sessionComparesBySession.get(id);
    if (!compare) {
      return badRequest('Comparison requires two completed traces.');
    }
    return json(compare);
  }

  if (/^\/api\/sessions\/[^/]+$/.test(url.pathname)) {
    const id = url.pathname.split('/').at(-1);
    const session = sessions.find((item) => item.id === id);
    return session ? json(session) : notFound();
  }

  return notFound();
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);

    // Proxy /docs/* to the Mintlify-hosted docs site when MINTLIFY_HOST is configured.
    // Set MINTLIFY_HOST (e.g. "continua.mintlify.dev") in Cloudflare Pages env vars to activate.
    const mintlifyHost = env?.MINTLIFY_HOST;
    if (mintlifyHost && (url.pathname === '/docs' || url.pathname.startsWith('/docs/'))) {
      const proxyUrl = new URL(url.pathname + url.search, `https://${mintlifyHost}`);
      const proxyRequest = new Request(proxyUrl, request);
      proxyRequest.headers.set('Host', mintlifyHost);
      proxyRequest.headers.set('X-Forwarded-Host', url.host);
      proxyRequest.headers.set('X-Forwarded-Proto', 'https');
      return fetch(proxyRequest);
    }

    if (url.pathname.startsWith('/api/')) {
      return handleApi(url);
    }

    const response = await env.ASSETS.fetch(request);
    if (response.status !== 404 || !request.headers.get('Accept')?.includes('text/html')) {
      return response;
    }

    return env.ASSETS.fetch(new URL('/index.html', request.url));
  },
};
