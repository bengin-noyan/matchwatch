-- Where the payment term came from in the contract (spec rule 3: contract-based findings cite page and clause).
ALTER TABLE contracts ADD COLUMN payment_term_source_page INT;
ALTER TABLE contracts ADD COLUMN payment_term_clause_text TEXT NOT NULL DEFAULT '';
-- NetUnitPrice divides by qty; the parser already rejects qty <= 0.
ALTER TABLE invoice_lines ADD CONSTRAINT invoice_lines_qty_positive CHECK (qty > 0);
