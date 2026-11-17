import json
from typing import Protocol

import httpx


class LLMError(Exception):
    """code: LLM_UNAVAILABLE | LLM_BAD_OUTPUT. Mesajlar asla belge metni ya da model çıktısı taşımaz."""

    def __init__(self, code: str, message: str = ""):
        super().__init__(message or code)
        self.code = code


class LLM(Protocol):
    model: str

    def complete_json(self, system: str, user: str, schema: dict, timeout_s=None) -> dict: ...


class OpenAICompatLLM:
    def __init__(self, base_url: str, model: str, timeout_s: int = 240,
                 transport: httpx.BaseTransport | None = None):
        self.base_url = base_url.rstrip("/")
        self.model = model
        self.timeout_s = timeout_s
        self.transport = transport

    def complete_json(self, system: str, user: str, schema: dict, timeout_s=None) -> dict:
        body = {
            "model": self.model,
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
            "temperature": 0,
            "response_format": {"type": "json_schema", "json_schema": {"name": "contract", "schema": schema}},
        }
        try:
            # trust_env=False: ortamdaki proxy değişkenleri sözleşme metnini asla başka yere yönlendirmemeli
            with httpx.Client(timeout=min(self.timeout_s, timeout_s or self.timeout_s), transport=self.transport,
                              trust_env=False) as c:
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


class OpenAICompatEmbedder:
    def __init__(self, base_url: str, model: str, timeout_s: int = 60,
                 transport: httpx.BaseTransport | None = None):
        self.base_url = base_url.rstrip("/")
        self.model = model
        self.timeout_s = timeout_s
        self.transport = transport

    def embed(self, texts: list[str]) -> list[list]:
        try:
            # trust_env=False: ortamdaki proxy değişkenleri kalem adlarını asla başka yere yönlendirmemeli
            with httpx.Client(timeout=self.timeout_s, transport=self.transport, trust_env=False) as c:
                r = c.post(f"{self.base_url}/v1/embeddings", json={"model": self.model, "input": texts})
        except httpx.HTTPError as e:
            raise LLMError("LLM_UNAVAILABLE", type(e).__name__) from None
        if r.status_code != 200:
            raise LLMError("LLM_UNAVAILABLE", f"HTTP {r.status_code}")
        try:
            data = sorted(r.json()["data"], key=lambda d: d["index"])
            vecs = [d["embedding"] for d in data]
        except (ValueError, KeyError, TypeError):
            raise LLMError("LLM_BAD_OUTPUT", "response is not an embedding list") from None
        ok = len(vecs) == len(texts) and all(
            isinstance(v, list) and v and all(isinstance(x, (int, float)) and not isinstance(x, bool) for x in v)
            for v in vecs)
        if not ok or len({len(v) for v in vecs}) != 1:
            raise LLMError("LLM_BAD_OUTPUT", "embedding count or shape mismatch")
        return vecs
