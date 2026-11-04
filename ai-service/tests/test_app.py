import logging

import pytest
from fastapi.testclient import TestClient

from matchwatch_ai.app import create_app
from matchwatch_ai.config import Settings
from matchwatch_ai.llm import LLMError
from matchwatch_ai.pdftext import MAX_BYTES
from conftest import FakeLLM

TOKEN = "t" * 32
AUTH = {"Authorization": f"Bearer {TOKEN}"}
PDF = {"Content-Type": "application/pdf"}
SETTINGS = Settings(token=TOKEN, llm_base_url="http://ollama:11434")


def make(answers):
    return TestClient(create_app(SETTINGS, llm=FakeLLM(answers), ocr=None))


@pytest.fixture
def client(good_answer):
    return make([good_answer])


def test_extract_ok(client, text_pdf):
    r = client.post("/v1/extract", content=text_pdf, headers=AUTH | PDF)
    assert r.status_code == 200 and r.json()["draft"]["contract_no"] == "SZL-2026-014"
    assert isinstance(r.json()["meta"]["duration_ms"], int)


def test_requires_token(client, text_pdf):
    assert client.post("/v1/extract", content=text_pdf, headers=PDF).status_code == 401
    r = client.post("/v1/extract", content=text_pdf, headers={"Authorization": "Bearer wrong"} | PDF)
    assert r.status_code == 401 and r.json()["code"] == "UNAUTHORIZED"


def test_not_pdf(client):
    r = client.post("/v1/extract", content=b"hello", headers=AUTH | PDF)
    assert r.status_code == 400 and r.json()["code"] == "NOT_PDF" and "error" in r.json()


def test_oversize_413_content_length(client):
    r = client.post("/v1/extract", content=b"x" * (MAX_BYTES + 1), headers=AUTH | PDF)
    assert r.status_code == 413 and r.json()["code"] == "TOO_LARGE"


def test_oversize_413_chunked(client):
    def gen():
        for _ in range(MAX_BYTES // 65536 + 2):
            yield b"x" * 65536
    r = client.post("/v1/extract", content=gen(), headers=AUTH | PDF)
    assert r.status_code == 413 and r.json()["code"] == "TOO_LARGE"


def test_llm_down_502(text_pdf):
    c = make([LLMError("LLM_UNAVAILABLE")])
    r = c.post("/v1/extract", content=text_pdf, headers=AUTH | PDF)
    assert r.status_code == 502 and r.json()["code"] == "LLM_UNAVAILABLE"


def test_no_content_in_logs(client, text_pdf, caplog):
    with caplog.at_level(logging.DEBUG):
        client.post("/v1/extract", content=text_pdf, headers=AUTH | PDF)
    assert "SZL-2026-014" not in caplog.text and "185,50" not in caplog.text
    assert "extract pages=" in caplog.text


def test_healthz_no_auth(client):
    r = client.get("/healthz")
    assert r.status_code == 200 and r.json() == {"status": "ok"}


def test_unexpected_error_500_no_leak(client, text_pdf, caplog, monkeypatch):
    def boom(*a, **k):
        raise RuntimeError("SZL-2026-014 secret")
    monkeypatch.setattr("matchwatch_ai.app.extract", boom)
    with caplog.at_level(logging.DEBUG):
        r = client.post("/v1/extract", content=text_pdf, headers=AUTH | PDF)
    assert r.status_code == 500 and r.json() == {"code": "INTERNAL", "error": "internal error"}
    assert "SZL-2026-014" not in caplog.text and "secret" not in caplog.text
    assert "RuntimeError" in caplog.text


def test_llm_error_text_not_echoed(text_pdf):
    c = make([LLMError("LLM_BAD_OUTPUT", "SZL-2026-014 raw model output")] * 5)
    r = c.post("/v1/extract", content=text_pdf, headers=AUTH | PDF)
    assert r.status_code == 502 and "SZL" not in r.text


def test_non_ascii_authorization_401(client, text_pdf):
    r = client.post("/v1/extract", content=text_pdf, headers={"Authorization": "Bearer ğüş".encode()} | PDF)
    assert r.status_code == 401
