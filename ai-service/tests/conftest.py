import json
from pathlib import Path

import pytest

FIX = Path(__file__).parent / "fixtures"


@pytest.fixture
def text_pdf() -> bytes:
    return (FIX / "contract_text.pdf").read_bytes()


@pytest.fixture
def scanned_pdf() -> bytes:
    return (FIX / "contract_scanned.pdf").read_bytes()


@pytest.fixture
def fixture_pages() -> list[str]:
    return json.loads((FIX / "contract_pages.json").read_text(encoding="utf-8"))
