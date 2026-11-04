import ipaddress
import os
from collections.abc import Mapping
from dataclasses import dataclass
from urllib.parse import urlparse


@dataclass
class Settings:
    token: str
    llm_base_url: str
    llm_model: str = "qwen2.5:7b-instruct"
    llm_timeout_s: int = 240
    max_chars: int = 24000


def is_private_host(host: str) -> bool:
    # shortcut: host adına bakar, DNS'e değil; TR bulutunda iç DNS adları (ör. ollama.internal) gerekirse izinli sonek listesi eklenir
    if not host:
        return False
    if host == "localhost" or "." not in host:
        return True
    try:
        ip = ipaddress.ip_address(host)
    except ValueError:
        return False
    return ip.is_private or ip.is_loopback


def load(env: Mapping[str, str] = os.environ) -> Settings:
    token = env.get("AI_SERVICE_TOKEN", "")
    if len(token) < 32:
        raise ValueError("AI_SERVICE_TOKEN must be at least 32 characters")
    url = env.get("LLM_BASE_URL", "")
    if not url or not is_private_host(urlparse(url).hostname or ""):
        raise ValueError("LLM_BASE_URL must be set and point to a private host (KVKK: no foreign LLM)")
    return Settings(
        token=token,
        llm_base_url=url,
        llm_model=env.get("LLM_MODEL", "qwen2.5:7b-instruct"),
        llm_timeout_s=int(env.get("LLM_TIMEOUT_S", "240")),
        max_chars=int(env.get("LLM_MAX_CHARS", "24000")),
    )
