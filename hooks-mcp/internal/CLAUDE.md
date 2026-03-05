# internal

Subpackages:
- dateparse/ — Human-friendly date range parsing → DateRange (unix timestamps + ISO strings)
- format/ — Pure formatting functions: Table, Tree, BarChart, FormatDuration, FormatCost, etc.
- meili/ — Searcher interface + hit types (shared contract between tools and backend)
- milvus/ — MilvusClient implementing Searcher via REST API v2 + Embedder for dense vectors
- tools/ — 11 MCP tool handlers + RegisterAll wiring
