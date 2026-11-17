-- Ödeme vadesinin sözleşmede nereden geldiği (spec kural 3: sözleşmeye dayanan bulgular sayfa ve maddeyi gösterir).
ALTER TABLE contracts ADD COLUMN payment_term_source_page INT;
ALTER TABLE contracts ADD COLUMN payment_term_clause_text TEXT NOT NULL DEFAULT '';
-- NetUnitPrice miktara bölüyor; parser qty <= 0 olanı zaten reddediyor.
ALTER TABLE invoice_lines ADD CONSTRAINT invoice_lines_qty_positive CHECK (qty > 0);
