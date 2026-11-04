"""Deterministic Turkish text parsers. Return None when unparseable; never raise."""
import datetime
import re
from decimal import Decimal, InvalidOperation

_MONTHS = ["ocak", "şubat", "mart", "nisan", "mayıs", "haziran", "temmuz", "ağustos", "eylül", "ekim", "kasım", "aralık"]


def _fold(s: str) -> str:
    # casefold() maps "I" to "i" not "ı", so unify dotless i for ALL-CAPS input
    return s.casefold().replace("ı", "i").replace("̇", "")


_MONTH_NO = {_fold(m): i + 1 for i, m in enumerate(_MONTHS)}


def parse_tr_number(text: str) -> tuple[Decimal | None, bool]:
    m = re.search(r"[\d.,]+", text)
    g = m.group().strip(".,") if m else ""
    if not re.search(r"\d", g):
        return None, False
    ambiguous = False
    if "." in g and "," in g:
        dec = "." if g.rfind(".") > g.rfind(",") else ","
        thou = "," if dec == "." else "."
        g = g.replace(thou, "").replace(dec, ".")
    elif "," in g:
        g = g.replace(",", "") if g.count(",") > 1 else g.replace(",", ".")
    elif "." in g:
        if g.count(".") > 1:
            g = g.replace(".", "")
        elif re.fullmatch(r"\d+\.\d{3}", g):
            g, ambiguous = g.replace(".", ""), True
    try:
        return Decimal(g), ambiguous
    except InvalidOperation:
        return None, False


_CURRENCIES = [
    ("TRY", r"₺|\btl\b|\btry\b|türk\s+lirasi"),
    ("USD", r"\$|\busd\b|abd\s+dolari"),
    ("EUR", r"€|\beur\b|\bavro\b|\beuro\b"),
    ("GBP", r"£|\bgbp\b|ingiliz\s+sterlini"),
]


def parse_currency(text: str) -> str | None:
    t = _fold(text)
    found = [code for code, pat in _CURRENCIES if re.search(pat, t)]
    return found[0] if len(found) == 1 else None  # two codes ("USD ... TL karşılığı") is ambiguous


def parse_date(text: str) -> str | None:
    t = _fold(text)
    m = re.search(r"(?<!\d)(\d{1,2})[./-](\d{1,2})[./-](\d{4})(?!\d)", t)
    if m:
        d, mo, y = map(int, m.groups())
    else:
        m = re.search(r"(\d{1,2})\s+([^\W\d]+)\s+(\d{4})", t)
        if not m or m.group(2) not in _MONTH_NO:
            return None
        d, mo, y = int(m.group(1)), _MONTH_NO[m.group(2)], int(m.group(3))
    try:
        return datetime.date(y, mo, d).isoformat()
    except ValueError:
        return None


def parse_days(text: str) -> int | None:
    m = re.search(r"(?<!\d)(\d{1,5})(?!\d)\s*\)?\s*gün", text.casefold())
    return int(m.group(1)) if m else None


def parse_percent(text: str) -> Decimal | None:
    m = re.search(r"(?:%\s*|yüzde\s+)(\d[\d.,]*)", text.casefold())
    return parse_tr_number(m.group(1))[0] if m else None


def vkn_valid(v: str) -> bool:
    if not (v.isascii() and v.isdigit()):
        return False
    d = [int(c) for c in v]
    if len(d) == 10:
        s = 0
        for i in range(9):
            t = (d[i] + 9 - i) % 10
            s += 9 if t == 9 else (t * 2 ** (9 - i)) % 9
        return d[9] == (10 - s % 10) % 10
    if len(d) == 11:
        if d[0] == 0:
            return False
        if (sum(d[0:9:2]) * 7 - sum(d[1:8:2])) % 10 != d[9]:
            return False
        return sum(d[:10]) % 10 == d[10]
    return False


_UOM = {"adet": "C62", "kg": "KGM", "kilogram": "KGM", "litre": "LTR", "lt": "LTR", "m": "MTR", "metre": "MTR",
        "ton": "TNE", "saat": "HUR", "ay": "MON"}


def map_uom(text: str) -> tuple[str, bool]:
    code = _UOM.get(text.strip().casefold())
    return (code, True) if code else (text, False)
