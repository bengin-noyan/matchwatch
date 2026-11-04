import pytest

from matchwatch_ai.config import load

T = "t" * 32


@pytest.mark.parametrize("url,ok", [("http://ollama:11434", True), ("http://localhost:11434", True),
    ("http://127.0.0.1:11434", True), ("http://10.0.3.4", True), ("http://192.168.1.2", True),
    ("https://api.openai.com", False), ("https://8.8.8.8", False), ("", False)])
def test_llm_url_gate(url, ok):
    env = {"AI_SERVICE_TOKEN": T, "LLM_BASE_URL": url}
    if ok:
        s = load(env)
        assert s.llm_model == "qwen2.5:7b-instruct" and s.llm_timeout_s == 240 and s.max_chars == 24000
    else:
        with pytest.raises(ValueError):
            load(env)


def test_short_token_rejected():
    with pytest.raises(ValueError):
        load({"AI_SERVICE_TOKEN": "t" * 31, "LLM_BASE_URL": "http://ollama:11434"})
