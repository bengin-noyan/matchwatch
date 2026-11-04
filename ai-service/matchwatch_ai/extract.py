"""Grounded contract extraction: the LLM only quotes; Python checks every quote is on its page and parses it."""
import re
import time

from .llm import LLM, LLMError
from .parsing import _fold, map_uom, parse_currency, parse_date, parse_days, parse_percent, parse_tr_number, vkn_valid
from .pdftext import Page

_HEADER = ["vendor_vkn", "vendor_name", "contract_no", "valid_from", "valid_until", "payment_term", "currency",
           "qty_tolerance"]
_ITEM_Q = ["name", "item_code", "barcode", "unit_price", "uom", "withholding_code", "clause"]
# warning field name = the draft field the quote fills
_FIELD = {"payment_term": "payment_term_days", "qty_tolerance": "qty_tolerance_pct"}

_Q = {"anyOf": [{"type": "object", "properties": {"text": {"type": "string"}, "page": {"type": "integer"}},
                 "required": ["text", "page"], "additionalProperties": False}, {"type": "null"}]}
_ITEM = {"type": "object", "properties": {**{k: _Q for k in _ITEM_Q}, "is_service": {"type": ["boolean", "null"]}},
         "required": [*_ITEM_Q, "is_service"], "additionalProperties": False}
QUOTE_SCHEMA = {"type": "object", "properties": {**{k: _Q for k in _HEADER}, "items": {"type": "array", "items": _ITEM}},
                "required": [*_HEADER, "items"], "additionalProperties": False}

SYSTEM = (
    "Sen bir sözleşme alıntılama aracısın. Verilen sözleşme sayfalarından istenen alanları JSON olarak döndür.\n"
    "Kurallar:\n"
    "- Her alan için yalnızca sözleşme metninde AYNEN, harfi harfine geçen ifadeyi alıntıla: "
    '{"text": "<aynen alıntı>", "page": <sayfa no>}.\n'
    "- Hesaplama, çevirme, biçim değiştirme, özetleme veya tahmin YAPMA. Sayıları yazıldığı gibi bırak.\n"
    "- Bir bilgi metinde yoksa ya da emin değilsen alanı null yap.\n"
    '- "page", alıntının geçtiği "=== SAYFA n ===" başlığındaki n numarasıdır.\n'
    '- contract_no ve vendor_name etiketsiz alıntılanır: "Sözleşme No:" veya "Satıcı:" gibi etiketi alma, '
    "yalnızca değeri al.\n"
    "- Tarihler: bir tarih aralığında her tarihi ayrı alıntıla, yalnızca tarihin kendisini al. valid_from "
    "başlangıç tarihi, valid_until bitiş tarihidir.\n"
    "- items: fiyat listesindeki her kalem için bir öğe; name kalem adı, unit_price birim fiyat ifadesi "
    "(para birimiyle birlikte), uom birim, clause kalemin geçtiği satır veya madde, withholding_code tevkifat "
    "kodu (yalnızca metinde kod olarak yazıyorsa), is_service kalem bir hizmetse true, malsa false, belli değilse null."
)

_REVIEW = re.compile(r"iskonto|indirim|fiyat farki|endeks")
_WHT = re.compile(r"6\d\d|8\d\d")


def _norm(s: str) -> str:
    return " ".join(s.casefold().split())


def _starts(n: str, hay: str) -> list[int]:
    out, i = [], hay.find(n)
    while i != -1:
        out.append(i)
        i = hay.find(n, i + 1)
    return out


def _numeric_ok(n: str, hay: str, i: int) -> bool:
    """False if the match at i is part of a longer number on the page ("85,50" inside "185,50").
    A trailing/leading [.,] counts only when a digit follows/precedes it, so a sentence-final "%5." stays ok."""
    pre, post = hay[:i], hay[i + len(n):]
    return not ((n[0] in "0123456789.," and re.search(r"\d[.,]?$", pre))
                or (n[-1] in "0123456789.," and re.match(r"[.,]?\d", post)))


def _negative(n: str, hay: str) -> bool:
    """A minus sign in the quote, or touching the quoted number on the page ("- 185,50" is a separator)."""
    return bool(re.search(r"[-−]\s?\d", n)) or (n[0].isdigit() and any(
        re.search(r"[-−]$", hay[:i]) for i in _starts(n, hay)))


def _is_q(v) -> bool:
    return v is None or (isinstance(v, dict) and isinstance(v.get("text"), str) and type(v.get("page")) is int)


def _valid(a) -> bool:
    if not isinstance(a, dict) or not all(_is_q(a.get(k)) for k in _HEADER):
        return False
    items = a.get("items") or []
    return isinstance(items, list) and all(
        isinstance(it, dict) and all(_is_q(it.get(k)) for k in _ITEM_Q)
        and isinstance(it.get("is_service"), (bool, type(None))) for it in items)


def _ask(llm: LLM, user: str, end, clock) -> dict:
    for attempt in (1, 2):
        left = end - clock()
        if left <= 0:
            raise LLMError("LLM_UNAVAILABLE", "extraction deadline exceeded")
        try:
            ans = llm.complete_json(SYSTEM, user, QUOTE_SCHEMA, timeout_s=left)
            if not _valid(ans):
                raise LLMError("LLM_BAD_OUTPUT", "response does not match schema")
            return ans
        except LLMError as e:
            if e.code != "LLM_BAD_OUTPUT" or attempt == 2:
                raise


def _groups(pages: list[Page], max_chars: int) -> list[list[Page]]:
    groups, size = [], 0
    for p in pages:
        if groups and size + len(p.text) <= max_chars:
            groups[-1].append(p)
            size += len(p.text)
        else:
            groups.append([p])
            size = len(p.text)
    return groups


_DATE = re.compile(r"(?<!\d)\d{1,2}[./-]\d{1,2}[./-]\d{4}(?!\d)|\d{1,2}\s+[^\W\d]+\s+\d{4}")


def _dates(t: str) -> list[str]:
    """All parseable dates in the quote, in order ("01.01.2026 – 31.12.2026" -> two)."""
    return [d for m in _DATE.finditer(t) if (d := parse_date(m.group()))]


def _header_values(k: str, t: str, page: int) -> tuple[dict | None, str]:
    """Parse a grounded header quote -> (draft values, "") or (None, warning code)."""
    if k == "vendor_vkn":
        v = re.sub(r"\D", "", t)
        return ({k: v}, "") if v else (None, "UNPARSEABLE")
    if k in ("vendor_name", "contract_no"):
        t = t.strip()
        c = t.find(":")
        # drop a "Sözleşme No:" / "Satıcı:" label, but keep colons inside values like "SZL:2026/14"
        if 0 <= c < 40 and (re.search(r"\s", t[:c]) or t[c + 1:c + 2].isspace()):
            t = t[c + 1:].strip()
        return ({k: t}, "") if t else (None, "UNPARSEABLE")
    if k in ("valid_from", "valid_until"):
        ds = _dates(t)
        if not ds:
            return None, "UNPARSEABLE"
        return {k: ds[1] if k == "valid_until" and len(ds) > 1 else ds[0]}, ""
    if k == "payment_term":
        d = parse_days(t)
        if d is None:
            return None, "UNPARSEABLE"
        return {"payment_term_days": d, "payment_term_source_page": page, "payment_term_clause_text": t.strip()}, ""
    if k == "currency":
        v = parse_currency(t)
        return ({k: v}, "") if v else (None, "UNKNOWN_CURRENCY")
    v = parse_percent(t)  # qty_tolerance; never defaulted to 0
    return ({"qty_tolerance_pct": str(v)}, "") if v is not None else (None, "UNPARSEABLE")


def extract(pages: list[Page], llm: LLM, max_chars: int = 24000, deadline_s=270,
            clock=time.monotonic) -> dict:
    end = clock() + deadline_s
    by_no = {p.number: p for p in pages}
    warnings: list[dict] = []

    def warn(code: str, field: str | None = None, page: int | None = None, message: str = "") -> dict:
        w = {"code": code, "field": field, "page": page, "message": message or code}
        warnings.append(w)
        return w

    def ground(q: dict | None, field: str, numeric: bool = False,
               message: str = "quote not found on the given page") -> str | None:
        """Quote text if it occurs on its page, else None (+ UNGROUNDED unless the LLM gave null).
        numeric: every occurrence must be a whole number on the page, never part of a longer one."""
        if q is None:
            return None
        p = by_no.get(q["page"])
        n = _norm(q["text"])
        if n and p is not None:
            hay = _norm(p.text)
            starts = _starts(n, hay)
            if starts and (not numeric or all(_numeric_ok(n, hay, i) for i in starts)):
                return q["text"]
            if starts:
                message = "quote is part of a longer number on the page"
        warn("UNGROUNDED", field, q["page"], message)
        return None

    def negative(q: dict) -> bool:
        return _negative(_norm(q["text"]), _norm(by_no[q["page"]].text))

    draft = {k: None for k in ["vendor_vkn", "vendor_name", "contract_no", "valid_from", "valid_until",
                               "payment_term_days", "payment_term_source_page", "payment_term_clause_text",
                               "currency", "qty_tolerance_pct"]}
    filled: set[str] = set()
    pending: dict[str, list[dict]] = {}  # header warnings, dropped if a later group fills the field
    items: list[dict] = []
    seen: set[tuple[str, str]] = set()

    for group in _groups(pages, max_chars):
        ans = _ask(llm, "\n\n".join(f"=== SAYFA {p.number} ===\n{p.text}" for p in group), end, clock)

        rng = None  # first grounded date-range quote of this group
        for k in _HEADER:
            q = ans.get(k)
            if q is None or k in filled:
                continue
            field = _FIELD.get(k, k)
            before = len(warnings)
            t = ground(q, field, numeric=k in ("vendor_vkn", "qty_tolerance"))
            if t is not None and k in ("valid_from", "valid_until") and len(ds := _dates(t)) > 1:
                rng = rng or ds
            vals, code = _header_values(k, t, q["page"]) if t is not None else (None, "")
            if code:
                warn(code, field, q["page"])
            if vals is None:
                pending.setdefault(field, []).extend(warnings[before:])
                del warnings[before:]
                continue
            filled.add(k)
            pending.pop(field, None)
            draft.update(vals)
            if k == "vendor_vkn" and not vkn_valid(vals[k]):
                warn("VKN_CHECKSUM", field, q["page"], "VKN checksum failed; value kept for review")
        if rng:  # a range quote fills a missing start, and a missing or not-after-start end
            if "valid_from" not in filled:
                filled.add("valid_from")
                pending.pop("valid_from", None)
                draft["valid_from"] = rng[0]
            if "valid_until" not in filled or draft["valid_until"] <= draft["valid_from"]:
                filled.add("valid_until")
                pending.pop("valid_until", None)
                draft["valid_until"] = rng[1]

        for j, it in enumerate(ans.get("items") or []):
            i = len(items)
            name_q, price_q = it.get("name"), it.get("unit_price")
            # dropped items are reported by the LLM's own index in this chunk's answer
            name = ground(name_q, f"items[{j}]", message="item dropped: name not found on the given page")
            if name is None:
                if name_q is None:
                    warn("UNGROUNDED", f"items[{j}]", None, "item dropped: no name")
                continue
            key = (_norm(name), _norm(price_q["text"]) if price_q else "")
            if key in seen:
                continue
            seen.add(key)
            f = f"items[{i}]."
            item = {"item_code": "", "barcode": "", "name": name.strip(), "unit_price": None, "currency": None,
                    "uom": None, "withholding_code": "", "requires_delivery": it.get("is_service") is not True,
                    "source_page": None, "source_clause_text": ""}
            for k in ("item_code", "barcode"):
                item[k] = (ground(it.get(k), f + k) or "").strip()
            t = ground(price_q, f + "unit_price", numeric=True)
            if t is not None:
                groups = len(re.findall(r"\d[\d.,]*", t))
                v, ambiguous = parse_tr_number(t) if groups == 1 else (None, False)
                if groups > 1:
                    warn("AMBIGUOUS_NUMBER", f + "unit_price", price_q["page"],
                         f"{f}unit_price: quote contains {groups} numbers, expected exactly one")
                elif v is not None and negative(price_q):
                    v = None
                    warn("UNPARSEABLE", f + "unit_price", price_q["page"], f"{f}unit_price: negative price")
                elif v is None:
                    warn("UNPARSEABLE", f + "unit_price", price_q["page"])
                else:
                    item["unit_price"] = str(v)
                    if ambiguous:
                        warn("AMBIGUOUS_NUMBER", f + "unit_price", price_q["page"], "thousands/decimal separator unclear")
                item["currency"] = parse_currency(t)
                item["source_page"] = price_q["page"]
                n = _norm(t)
                item["source_clause_text"] = next(
                    (ln.strip() for ln in by_no[price_q["page"]].text.splitlines() if n in _norm(ln)), t.strip())
            t = ground(it.get("uom"), f + "uom")
            if t is not None:
                item["uom"], known = map_uom(t)
                if not known:
                    warn("UNKNOWN_UOM", f + "uom", it["uom"]["page"], "unit not mapped to a UN/ECE code")
            t = ground(it.get("withholding_code"), f + "withholding_code")
            if t is not None and _WHT.fullmatch(t.strip()):
                item["withholding_code"] = t.strip()
            t = ground(it.get("clause"), f + "clause")
            if t is not None:
                item["source_clause_text"] = t.strip()
                if item["source_page"] is None:
                    item["source_page"] = it["clause"]["page"]
            items.append(item)

    for ws in pending.values():
        warnings.extend(ws)
    if draft["valid_from"] and draft["valid_until"] and draft["valid_until"] <= draft["valid_from"]:
        draft["valid_until"] = None
        warn("UNPARSEABLE", "valid_until", None, "valid_until: end date is not after the start date")
    for item in items:
        item["currency"] = item["currency"] or draft["currency"]
    for p in pages:
        if _REVIEW.search(_fold(p.text)):
            warn("PRICE_CLAUSE_REVIEW", None, p.number, "page mentions a discount or price-adjustment clause")
        if p.ocr:
            warn("OCR_USED", None, p.number, "page text came from OCR")
    draft["items"] = items
    return {"draft": draft, "warnings": warnings,
            "meta": {"pages": len(pages), "ocr_pages": [p.number for p in pages if p.ocr], "model": llm.model}}
