# cmd/hooks-mcp

Entry point. Reads env vars (MILVUS_URL, MILVUS_TOKEN, EMBED_SVC_URL, collection names), health-checks Milvus via collections/list endpoint, creates MilvusClient with Embedder, creates MCP server, registers all 11 tools, runs on stdio transport. All logging to stderr.
