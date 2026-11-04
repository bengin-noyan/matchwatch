from decimal import Decimal

import pytest

from matchwatch_ai.parsing import (map_uom, parse_currency, parse_date, parse_days,
                                   parse_percent, parse_tr_number, vkn_valid)

D = Decimal


@pytest.mark.parametrize("s,val,amb", [
    ("1.250,00", D("1250.00"), False), ("1250,5", D("1250.5"), False), ("12,50 TL", D("12.50"), False),
    ("185,50", D("185.50"), False), ("1.250", D("1250"), True), ("1.234.567,8", D("1234567.8"), False),
    ("0,0375", D("0.0375"), False), ("12.5", D("12.5"), False), ("1,250.00", D("1250.00"), False),
    ("abc", None, False), ("", None, False)])
def test_parse_tr_number(s, val, amb):
    assert parse_tr_number(s) == (val, amb)


@pytest.mark.parametrize("s,c", [("12,50 TL", "TRY"), ("₺10", "TRY"), ("Türk Lirası", "TRY"), ("TRY", "TRY"),
    ("$5", "USD"), ("ABD Doları", "USD"), ("€3", "EUR"), ("Avro", "EUR"), ("Euro", "EUR"), ("GBP 4", "GBP"), ("10", None),
    ("ABD Doları (TL karşılığı)", None), ("USD olup TL karşılığı ödenir", None)])
def test_parse_currency(s, c):
    assert parse_currency(s) == c


@pytest.mark.parametrize("s,d", [("01.01.2026", "2026-01-01"), ("31/12/2026", "2026-12-31"),
    ("1 Ocak 2026", "2026-01-01"), ("15 Şubat 2026", "2026-02-15"), ("31 Aralık 2026", "2026-12-31"),
    ("31.02.2026", None), ("yarın", None)])
def test_parse_date(s, d):
    assert parse_date(s) == d


@pytest.mark.parametrize("s,n", [("60 gün", 60), ("altmış (60) gün", 60), ("fatura tarihinden itibaren 30 gün", 30),
    ("altmış gün", None), ("peşin", None)])
def test_parse_days(s, n):
    assert parse_days(s) == n


@pytest.mark.parametrize("s,p", [("%5", D("5")), ("% 5", D("5")), ("yüzde 2,5", D("2.5")), ("5", None)])
def test_parse_percent(s, p):
    assert parse_percent(s) == p


def test_vkn_valid():
    assert vkn_valid("1234567890") and vkn_valid("9876543217") and vkn_valid("10000000146")
    assert not vkn_valid("1234567891") and not vkn_valid("123") and not vkn_valid("01000000146")


@pytest.mark.parametrize("s,r", [("adet", ("C62", True)), ("Adet", ("C62", True)), ("kg", ("KGM", True)),
    ("kilogram", ("KGM", True)), ("litre", ("LTR", True)), ("lt", ("LTR", True)), ("m", ("MTR", True)),
    ("ton", ("TNE", True)), ("saat", ("HUR", True)), ("ay", ("MON", True)), ("koli", ("koli", False))])
def test_map_uom(s, r):
    assert map_uom(s) == r


@pytest.mark.parametrize("s,c", [("TÜRK LİRASI", "TRY"), ("ABD DOLARI", "USD")])
def test_currency_uppercase_turkish(s, c):
    assert parse_currency(s) == c


@pytest.mark.parametrize("s,d", [("15 EKİM 2026", "2026-10-15"), ("1 HAZİRAN 2026", "2026-06-01"),
                                 ("15 NİSAN 2026", "2026-04-15"), ("101.01.2026", None), ("01.01.20261", None)])
def test_date_regressions(s, d):
    assert parse_date(s) == d


def test_parse_days_huge_digits_no_raise():
    assert parse_days("9" * 5000 + " gün") is None
