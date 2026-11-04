# matchwatch ai-service

Stateless PDF contract extraction. `POST /v1/extract` (raw PDF body, `Authorization: Bearer <token>`) returns `{draft, warnings, meta}`; `GET /healthz`.

Env: `AI_SERVICE_TOKEN` (>= 32 chars), `LLM_BASE_URL` (OpenAI-compatible, private host only), `LLM_MODEL`, `LLM_TIMEOUT_S`, `LLM_MAX_CHARS`.

    docker build -t matchwatch-ai .
    docker run -e AI_SERVICE_TOKEN=... -e LLM_BASE_URL=http://ollama:11434 -p 8090:8090 matchwatch-ai

KVKK: documents are processed in memory and never stored; document text, quotes and values are never logged. Startup refuses a non-private `LLM_BASE_URL`, so contract text cannot go to a foreign LLM API.
