import json
import logging

import httpx
import pytest
from fastapi.testclient import TestClient

from matchwatch_ai.app import create_app
from matchwatch_ai.config import Settings
from matchwatch_ai.llm import LLMError, OpenAICompatEmbedder
from conftest import FakeLLM

TOKEN = "t" * 32
AUTH = {"Authorization": f"Bearer {TOKEN}"}
SETTINGS = Settings(token=TOKEN, llm_base_url="http://ollama:11434")


class FakeEmbedder:
    model = "fake-embed"

    def __init__(self, err=None):
        self.err, self.calls = err, []

    def embed(self, texts):
        self.calls.append(texts)
        if self.err:
            raise self.err
        return [[len(t), 1] for t in texts]


def client(emb):
    return TestClient(create_app(SETTINGS, llm=FakeLLM([]), ocr=None, embedder=emb))


def test_embed_ok_keeps_order():
    r = client(FakeEmbedder()).post("/v1/embed", json={"texts": ["ab", "abcd"]}, headers=AUTH)
    assert r.status_code == 200 and r.json() == {"model": "fake-embed", "vectors": [[2, 1], [4, 1]]}


def test_embed_requires_token():
    r = client(FakeEmbedder()).post("/v1/embed", json={"texts": ["a"]})
    assert r.status_code == 401 and r.json()["code"] == "UNAUTHORIZED"


@pytest.mark.parametrize("body", [
    {"texts": []}, {"texts": ["a"] * 257}, {"texts": [""]}, {"texts": ["  "]}, {"texts": ["x" * 513]},
    {"texts": [1]}, {"texts": "a"}, {}, [],
])
def test_embed_bad_request(body):
    emb = FakeEmbedder()
    r = client(emb).post("/v1/embed", json=body, headers=AUTH)
    assert r.status_code == 400 and r.json()["code"] == "BAD_REQUEST" and emb.calls == []


def test_embed_not_json():
    r = client(FakeEmbedder()).post("/v1/embed", content=b"nope", headers=AUTH)
    assert r.status_code == 400


def test_embed_upstream_down_502_no_names_logged(caplog):
    with caplog.at_level(logging.DEBUG):
        r = client(FakeEmbedder(LLMError("LLM_UNAVAILABLE"))).post(
            "/v1/embed", json={"texts": ["Ayçiçek yağı"]}, headers=AUTH)
    assert r.status_code == 502 and r.json()["code"] == "LLM_UNAVAILABLE"
    assert "Ayçiçek" not in caplog.text and "embed n=1" in caplog.text


def embedder(handler):
    return OpenAICompatEmbedder("http://llm:11434", "bge-m3", transport=httpx.MockTransport(handler))


def test_embedder_request_and_index_order():
    seen = {}

    def handler(req):
        seen["url"], seen["body"] = str(req.url), json.loads(req.content)
        return httpx.Response(200, json={"data": [{"index": 1, "embedding": [0.5, 2]},
                                                  {"index": 0, "embedding": [1, -0.25]}]})

    assert embedder(handler).embed(["a", "b"]) == [[1, -0.25], [0.5, 2]]
    assert seen["url"] == "http://llm:11434/v1/embeddings"
    assert seen["body"] == {"model": "bge-m3", "input": ["a", "b"]}


@pytest.mark.parametrize("resp,code", [
    (httpx.Response(500), "LLM_UNAVAILABLE"),
    (httpx.Response(200, content=b"x"), "LLM_BAD_OUTPUT"),
    (httpx.Response(200, json={"data": [{"index": 0, "embedding": [1]}]}), "LLM_BAD_OUTPUT"),  # sayı
    (httpx.Response(200, json={"data": [{"index": 0, "embedding": [1]}, {"index": 1, "embedding": [1, 2]}]}),
     "LLM_BAD_OUTPUT"),  # boyları farklı
    (httpx.Response(200, json={"data": [{"index": 0, "embedding": ["1"]}, {"index": 1, "embedding": [1]}]}),
     "LLM_BAD_OUTPUT"),  # sayı değil
    (httpx.Response(200, json={"data": [{"index": 0, "embedding": []}, {"index": 1, "embedding": []}]}),
     "LLM_BAD_OUTPUT"),  # boş
])
def test_embedder_errors(resp, code):
    with pytest.raises(LLMError) as e:
        embedder(lambda req: resp).embed(["a", "b"])
    assert e.value.code == code


def test_embedder_connect_error():
    def handler(req):
        raise httpx.ConnectError("boom")
    with pytest.raises(LLMError) as e:
        embedder(handler).embed(["a"])
    assert e.value.code == "LLM_UNAVAILABLE"


def test_embed_body_too_large():
    emb = FakeEmbedder()
    r = client(emb).post("/v1/embed", content=b'{"texts":["' + b"x" * (1 << 20) + b'"]}', headers=AUTH)
    assert r.status_code == 413 and r.json()["code"] == "TOO_LARGE" and emb.calls == []
