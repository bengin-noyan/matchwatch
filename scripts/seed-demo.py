"""Bir geliştirme tenant'ını birkaç aylık gerçekçi belgeyle doldurur, böylece web uygulamasının grafiklerinde gösterecek bir şey olur.

Yalnızca geliştirme için. Kullanım (API çalışıyor, hesap cmd/onboard ile açılmış):
    python -X utf8 -I scripts/seed-demo.py http://127.0.0.1:8080 demo@matchwatch.local demo-sifre-2026

Sözleşmeli üç tedarikçi; Temmuz'dan Kasım 2026 başına kadar haftalık irsaliyeler ve faturalar. Faturaların
yaklaşık dörtte birinde fiyat sözleşmenin üstünde, bazılarında vade daha kısa, bazılarında teslim edilenden fazla
miktar var. Her seferinde aynı veriyi üretir (sabit rastgele tohum); yeniden çalıştırmak yeni ETTN'lerle yeni bir
parti ekler.
"""
import json
import random
import sys
import urllib.request
import uuid
from datetime import date, timedelta
from decimal import Decimal
from http.cookiejar import CookieJar

BUYER_VKN, BUYER = "9876543210", "Yılmaz Makine San. Tic. Ltd. Şti."
NS = ('xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" '
      'xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"')

VENDORS = [
    {"vkn": "2345678901", "name": "Anadolu Ambalaj San. A.Ş.", "contract_no": "AMB-2026-07", "term": 45, "tol": "3.00",
     "items": [("KOLI-403030", "Koli 40x30x30 cm", "8.500000", (400, 1200)),
               ("STREC-50", "Streç film 50 cm", "145.000000", (10, 40))]},
    {"vkn": "3456789012", "name": "Ege Endüstriyel Yağ Ltd. Şti.", "contract_no": "EGE-2026-02", "term": 30, "tol": "0.00",
     "items": [("YAG-HID46", "Hidrolik yağ 46 (18 L)", "2350.000000", (4, 16)),
               ("YAG-GRES", "Gres yağı 15 kg", "1890.000000", (2, 8))]},
    {"vkn": "1234567890", "name": "Demir Bağlantı A.Ş.", "contract_no": "S-2026-001", "term": 60, "tol": "2.00",
     "items": [("CV-M8-20", "M8x20 paslanmaz civata", "1.250000", (2000, 9000))]},
]


class API:
    def __init__(self, base):
        self.base = base.rstrip("/") + "/api/v1"
        self.http = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(CookieJar()))

    def call(self, method, path, body=None, ctype="application/json"):
        data = body if isinstance(body, bytes) else (json.dumps(body).encode() if body is not None else None)
        req = urllib.request.Request(self.base + path, data=data, method=method, headers={"Content-Type": ctype})
        try:
            with self.http.open(req, timeout=60) as r:
                return r.status, json.loads(r.read() or b"{}")
        except urllib.error.HTTPError as e:
            return e.code, json.loads(e.read() or b"{}")


def party(tag, vkn, name):
    return (f"<cac:{tag}><cac:Party><cac:PartyIdentification><cbc:ID schemeID=\"VKN\">{vkn}</cbc:ID>"
            f"</cac:PartyIdentification><cac:PartyName><cbc:Name>{name}</cbc:Name></cac:PartyName></cac:Party></cac:{tag}>")


def despatch_xml(no, day, vendor, lines):
    body = "".join(
        f"<cac:DespatchLine><cbc:ID>{i}</cbc:ID><cbc:DeliveredQuantity unitCode=\"C62\">{qty}</cbc:DeliveredQuantity>"
        f"<cac:Item><cbc:Name>{name}</cbc:Name><cac:SellersItemIdentification><cbc:ID>{code}</cbc:ID>"
        f"</cac:SellersItemIdentification></cac:Item></cac:DespatchLine>"
        for i, (code, name, qty) in enumerate(lines, 1))
    return (f'<?xml version="1.0" encoding="UTF-8"?><DespatchAdvice xmlns="urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2" {NS}>'
            f"<cbc:UBLVersionID>2.1</cbc:UBLVersionID><cbc:CustomizationID>TR1.2</cbc:CustomizationID>"
            f"<cbc:ProfileID>TEMELIRSALIYE</cbc:ProfileID><cbc:ID>{no}</cbc:ID><cbc:UUID>{uuid.uuid4()}</cbc:UUID>"
            f"<cbc:IssueDate>{day}</cbc:IssueDate><cbc:DespatchAdviceTypeCode>SEVK</cbc:DespatchAdviceTypeCode>"
            f"{party('DespatchSupplierParty', vendor['vkn'], vendor['name'])}{party('DeliveryCustomerParty', BUYER_VKN, BUYER)}"
            f"<cac:Shipment><cbc:ID>1</cbc:ID><cac:Delivery><cac:Despatch><cbc:ActualDespatchDate>{day}</cbc:ActualDespatchDate>"
            f"</cac:Despatch></cac:Delivery></cac:Shipment>{body}</DespatchAdvice>")


def money(d):
    return f"{d.quantize(Decimal('0.01'))}"


def invoice_xml(no, day, due, vendor, despatch_no, despatch_day, lines):
    total = sum((q * p for _, _, q, p in lines), Decimal(0))
    vat = total * Decimal("0.20")
    body = "".join(
        f"<cac:InvoiceLine><cbc:ID>{i}</cbc:ID><cbc:InvoicedQuantity unitCode=\"C62\">{q}</cbc:InvoicedQuantity>"
        f"<cbc:LineExtensionAmount currencyID=\"TRY\">{money(q * p)}</cbc:LineExtensionAmount>"
        f"<cac:TaxTotal><cbc:TaxAmount currencyID=\"TRY\">{money(q * p * Decimal('0.20'))}</cbc:TaxAmount>"
        f"<cac:TaxSubtotal><cbc:TaxableAmount currencyID=\"TRY\">{money(q * p)}</cbc:TaxableAmount>"
        f"<cbc:TaxAmount currencyID=\"TRY\">{money(q * p * Decimal('0.20'))}</cbc:TaxAmount><cbc:Percent>20</cbc:Percent>"
        f"<cac:TaxCategory><cac:TaxScheme><cbc:Name>KDV</cbc:Name><cbc:TaxTypeCode>0015</cbc:TaxTypeCode></cac:TaxScheme>"
        f"</cac:TaxCategory></cac:TaxSubtotal></cac:TaxTotal>"
        f"<cac:Item><cbc:Name>{name}</cbc:Name><cac:SellersItemIdentification><cbc:ID>{code}</cbc:ID>"
        f"</cac:SellersItemIdentification></cac:Item>"
        f"<cac:Price><cbc:PriceAmount currencyID=\"TRY\">{p}</cbc:PriceAmount></cac:Price></cac:InvoiceLine>"
        for i, (code, name, q, p) in enumerate(lines, 1))
    return (f'<?xml version="1.0" encoding="UTF-8"?><Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" {NS}>'
            f"<cbc:UBLVersionID>2.1</cbc:UBLVersionID><cbc:CustomizationID>TR1.2</cbc:CustomizationID>"
            f"<cbc:ProfileID>TICARIFATURA</cbc:ProfileID><cbc:ID>{no}</cbc:ID><cbc:UUID>{uuid.uuid4()}</cbc:UUID>"
            f"<cbc:IssueDate>{day}</cbc:IssueDate><cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>"
            f"<cbc:DocumentCurrencyCode>TRY</cbc:DocumentCurrencyCode>"
            f"<cac:DespatchDocumentReference><cbc:ID>{despatch_no}</cbc:ID><cbc:IssueDate>{despatch_day}</cbc:IssueDate>"
            f"</cac:DespatchDocumentReference>"
            f"{party('AccountingSupplierParty', vendor['vkn'], vendor['name'])}{party('AccountingCustomerParty', BUYER_VKN, BUYER)}"
            f"<cac:PaymentMeans><cbc:PaymentMeansCode>1</cbc:PaymentMeansCode><cbc:PaymentDueDate>{due}</cbc:PaymentDueDate></cac:PaymentMeans>"
            f"<cac:TaxTotal><cbc:TaxAmount currencyID=\"TRY\">{money(vat)}</cbc:TaxAmount></cac:TaxTotal>"
            f"<cac:LegalMonetaryTotal><cbc:LineExtensionAmount currencyID=\"TRY\">{money(total)}</cbc:LineExtensionAmount>"
            f"<cbc:TaxExclusiveAmount currencyID=\"TRY\">{money(total)}</cbc:TaxExclusiveAmount>"
            f"<cbc:TaxInclusiveAmount currencyID=\"TRY\">{money(total + vat)}</cbc:TaxInclusiveAmount>"
            f"<cbc:PayableAmount currencyID=\"TRY\">{money(total + vat)}</cbc:PayableAmount></cac:LegalMonetaryTotal>"
            f"{body}</Invoice>")


def main():
    base, email, password = sys.argv[1:4]
    api = API(base)
    st, me = api.call("POST", "/auth/login", {"email": email, "password": password})
    if st != 200:
        sys.exit(f"login failed: {st} {me}")
    print("tenant:", me["tenant_name"])
    rnd = random.Random(2026)
    run = uuid.uuid4().hex[:4].upper()

    for v in VENDORS:
        doc = {"vendor_vkn": v["vkn"], "vendor_name": v["name"], "contract_no": v["contract_no"],
               "valid_from": "2026-01-01", "valid_until": "2026-12-31", "payment_term_days": v["term"],
               "currency": "TRY", "qty_tolerance_pct": v["tol"], "payment_term_source_page": 2,
               "payment_term_clause_text": f"Madde 5: Ödeme vadesi fatura tarihinden itibaren {v['term']} gündür.",
               "items": [{"item_code": c, "name": n, "unit_price": p, "currency": "TRY", "uom": "C62",
                          "source_page": 3, "source_clause_text": f"Ek-1: {n} birim fiyatı {Decimal(p).normalize()} TL + KDV."}
                         for c, n, p, _ in v["items"]]}
        st, r = api.call("POST", "/contracts", doc)
        print(f"contract {v['contract_no']}: {st}" + ("" if st == 201 else f" {r.get('code')}"))

    n = 0
    dispute = []
    week = date(2026, 7, 6)
    while week <= date(2026, 11, 2):
        for v in rnd.sample(VENDORS, rnd.randint(1, 3)):
            n += 1
            day = week + timedelta(days=rnd.randint(0, 4))
            items = rnd.sample(v["items"], rnd.randint(1, len(v["items"])))
            invoiced, delivered = [], []
            for code, name, price, (lo, hi) in items:
                qty = Decimal(rnd.randrange(lo, hi, 10 if hi > 100 else 1))
                p = Decimal(price)
                if rnd.random() < 0.27:  # sözleşmenin üstünde sessiz bir fiyat artışı
                    p = (p * (1 + Decimal(rnd.choice([2, 4, 5, 8, 12])) / 100)).quantize(Decimal("0.01"))
                invoiced.append((code, name, qty, p.quantize(Decimal("0.01")) if p >= 1 else p))
                short = rnd.random() < 0.15
                delivered.append((code, name, (qty * Decimal("0.9")).quantize(Decimal("1")) if short else qty))
            term = v["term"] if rnd.random() > 0.15 else v["term"] // 2
            dno, ino = f"IRS{run}{n:09d}", f"FTR{run}{n:09d}"
            dday = day - timedelta(days=2)
            st, r = api.call("POST", "/despatches", despatch_xml(dno, dday, v, delivered).encode(), "application/xml")
            if st != 201:
                sys.exit(f"despatch {dno}: {st} {r}")
            st, r = api.call("POST", "/invoices",
                             invoice_xml(ino, day, day + timedelta(days=term), v, dno, dday, invoiced).encode(), "application/xml")
            if st != 201:
                sys.exit(f"invoice {ino}: {st} {r}")
            if r.get("audit_status") == "RED" and rnd.random() < 0.3:
                dispute.append(r["id"])
        week += timedelta(days=7)

    disputed = 0
    for inv in dispute:  # bazılarına zaten itiraz edilmiş olsun ki "itiraz edildi" rakamları hep sıfır görünmesin
        _, d = api.call("GET", f"/invoices/{inv}")
        for f in d.get("findings", []):
            if f["rule_code"] == "PRICE_MISMATCH":
                api.call("PATCH", f"/findings/{f['id']}", {"resolution_status": "DISPUTED",
                                                           "note": "Tedarikçiye fiyat farkı yazıyla bildirildi."})
                disputed += 1
    print(f"invoices: {n}, disputed findings: {disputed}")


if __name__ == "__main__":
    main()
