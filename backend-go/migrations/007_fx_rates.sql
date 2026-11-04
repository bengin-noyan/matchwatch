-- Faz 2c (D17): TCMB daily bulletins, global (not tenant data, like withholding_codes).
-- fx_days also records days without a bulletin (weekends, holidays) so they are not fetched again.
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
