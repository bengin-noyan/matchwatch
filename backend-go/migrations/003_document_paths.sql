-- Ham belgelerin yeri (STORAGE_DIR/<tenant>/<ettn>.xml); boş olabilir. RLS ve yetkiler bu tabloları zaten kapsıyor.
ALTER TABLE invoices ADD COLUMN raw_xml_path TEXT;
ALTER TABLE despatch_advices ADD COLUMN raw_xml_path TEXT;
ALTER TABLE contracts ADD COLUMN file_path TEXT;
