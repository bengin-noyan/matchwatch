import json
from typing import Protocol

import httpx


class LLMError(Exception):
    """code: LLM_UNAVAILABLE | LLM_BAD_OUTPUT. Messages never carry document text or model output."""

    def __init__(self, code: str, message: str = ""):
        super().__init__(message or code)
        self.code = code


class LLM(Protocol):
    model: str

    def complete_json(self, system: str, user: str, schema: dict) -> dict: ...


class OpenAICompatLLM:
    def __init__(self, base_url: str, model: str, timeout_s: int = 240,
                 transport: httpx.BaseTransport | None = None):
        self.base_url = base_url.rstrip("/")
        self.model = model
        self.timeout_s = timeout_s
        self.transport = transport

    def complete_json(self, system: str, user: str, schema: dict) -> dict:
        body = {
            "model": self.model,
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
            "temperature": 0,
            "response_format": {"type": "json_schema", "json_schema": {"name": "contract", "schema": schema}},
        }
        try:
            with httpx.Client(timeout=self.timeout_s, transport=self.transport) as c:
                r = c.post(f"{self.base_url}/v1/chat/completions", json=body)
        except httpx.HTTPError as e:
            raise LLMError("LLM_UNAVAILABLE", type(e).__name__) from None
        if r.status_code != 200:
            raise LLMError("LLM_UNAVAILABLE", f"HTTP {r.status_code}")
        try:
            out = json.loads(r.json()["choices"][0]["message"]["content"])
        except (ValueError, KeyError, IndexError, TypeError):
            raise LLMError("LLM_BAD_OUTPUT", "response is not JSON") from None
        if not isinstance(out, dict):
            raise LLMError("LLM_BAD_OUTPUT", "response is not a JSON object")
        return out
