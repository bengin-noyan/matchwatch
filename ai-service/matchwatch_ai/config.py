import ipaddress
import os
import re
import socket
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
    if host.startswith("[") and host.endswith("]"):
        host = host[1:-1]
    try:  # inet_aton also parses decimal/hex/octal forms (2852039166, 0xa9fea9fe) the way getaddrinfo does
        ip = ipaddress.IPv4Address(socket.inet_aton(host))
    except (OSError, ValueError):
        try:
            ip = ipaddress.ip_address(host)
        except ValueError:
            return host == "localhost" or re.fullmatch(r"[a-z][a-z0-9-]*", host.lower()) is not None
    return (ip.is_private or ip.is_loopback) and not ip.is_link_local


def load(env: Mapping[str, str] = os.environ) -> Settings:
    token = env.get("AI_SERVICE_TOKEN", "")
    if len(token) < 32:
        raise ValueError("AI_SERVICE_TOKEN must be at least 32 characters")
    url = env.get("LLM_BASE_URL", "").strip()
    if urlparse(url).scheme not in ("http", "https") or not is_private_host(urlparse(url).hostname or ""):
        raise ValueError("LLM_BASE_URL must be set and point to a private host (KVKK: no foreign LLM)")
    return Settings(
        token=token,
        llm_base_url=url,
        llm_model=env.get("LLM_MODEL", "qwen2.5:7b-instruct"),
        llm_timeout_s=int(env.get("LLM_TIMEOUT_S", "240")),
        max_chars=int(env.get("LLM_MAX_CHARS", "24000")),
    )
