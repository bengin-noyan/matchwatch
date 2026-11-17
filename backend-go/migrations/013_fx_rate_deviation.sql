-- D18: faturanın kendi kuru (PricingExchangeRate) TCMB döviz alış kurundan %1'den fazla sapıyor.
ALTER TABLE audit_findings DROP CONSTRAINT audit_findings_rule_code_check,
  ADD CONSTRAINT audit_findings_rule_code_check CHECK (rule_code IN ('PRICE_MISMATCH','QTY_EXCEEDS_DELIVERY',
    'UNCONTRACTED_ITEM','MATURITY_VIOLATION','DUPLICATE_SUSPECT','WITHHOLDING_ERROR','PRICE_CREEP','FX_UNVERIFIED',
    'UOM_MISMATCH','FX_RATE_DEVIATION'));
