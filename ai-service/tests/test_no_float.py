import pathlib
import re

PAT = re.compile(r"\bfloat\s*\(|:\s*float\b|->\s*float\b")


def test_no_float():
    root = pathlib.Path(__file__).parent.parent / "matchwatch_ai"
    bad = [str(p) for p in root.rglob("*.py") if PAT.search(p.read_text(encoding="utf-8"))]
    assert not bad, bad
