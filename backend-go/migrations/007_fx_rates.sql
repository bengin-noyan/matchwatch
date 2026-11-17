-- Faz 2c (D17): TCMB günlük bültenleri. Global tablolar, tenant verisi değil (withholding_codes gibi). fx_days bülten
-- olmayan günleri de (hafta sonu, tatil) kaydeder ki tekrar çekilmesinler.
CREATE TABLE fx_days (
  rate_date  DATE PRIMARY KEY,
  published  BOOLEAN NOT NULL,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE fx_rates (
  rate_date    DATE NOT NULL REFERENCES fx_days(rate_date),
  currency     VARCHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  unit         INT NOT NULL CHECK (unit > 0),
  forex_buying NUMERIC(19,6) NOT NULL CHECK (forex_buying > 0),
  PRIMARY KEY (rate_date, currency)
);

GRANT SELECT, INSERT ON fx_days, fx_rates TO matchwatch_app;
