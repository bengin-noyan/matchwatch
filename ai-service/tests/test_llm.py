import json

import httpx
import pytest

from matchwatch_ai.llm import LLMError, OpenAICompatLLM


def make(handler):
    return OpenAICompatLLM("http://llm:11434", "m1", transport=httpx.MockTransport(handler))


def reply(content):
    return httpx.Response(200, json={"choices": [{"message": {"content": content}}]})


def test_request_and_parse():
    seen = {}

    def handler(req):
        seen["url"] = str(req.url)
        seen["body"] = json.loads(req.content)
        return reply('{"contract_no": null}')

    assert make(handler).complete_json("sys", "usr", {"type": "object"}) == {"contract_no": None}
    b = seen["body"]
    assert seen["url"] == "http://llm:11434/v1/chat/completions"
    assert b["temperature"] == 0 and b["model"] == "m1"
    assert b["response_format"] == {"type": "json_schema",
                                    "json_schema": {"name": "contract", "schema": {"type": "object"}}}
    assert b["messages"] == [{"role": "system", "content": "sys"}, {"role": "user", "content": "usr"}]


def _raise(exc):
    def handler(req):
        raise exc
    return handler


@pytest.mark.parametrize("handler,code", [
    (lambda r: httpx.Response(503), "LLM_UNAVAILABLE"),
    (_raise(httpx.ConnectError("down")), "LLM_UNAVAILABLE"),
    (_raise(httpx.ReadTimeout("slow")), "LLM_UNAVAILABLE"),
    (lambda r: reply("not json"), "LLM_BAD_OUTPUT"),
    (lambda r: reply("[1]"), "LLM_BAD_OUTPUT"),
    (lambda r: httpx.Response(200, json={"choices": []}), "LLM_BAD_OUTPUT"),
])
def test_errors(handler, code):
    with pytest.raises(LLMError) as e:
        make(handler).complete_json("s", "u", {})
    assert e.value.code == code


def test_client_ignores_proxy_env_and_caps_timeout(monkeypatch):
    seen = {}
    real = httpx.Client

    def spy(**kw):
        seen.update(kw)
        return real(**kw)

    monkeypatch.setattr(httpx, "Client", spy)
    make(lambda r: reply("{}")).complete_json("s", "u", {}, timeout_s=7)
    assert seen["trust_env"] is False and seen["timeout"] == 7
