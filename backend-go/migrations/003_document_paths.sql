-- Raw document locations (STORAGE_DIR/<tenant>/<ettn>.xml); nullable, RLS and grants already cover these tables.
ALTER TABLE invoices ADD COLUMN raw_xml_path TEXT;
ALTER TABLE despatch_advices ADD COLUMN raw_xml_path TEXT;
ALTER TABLE contracts ADD COLUMN file_path TEXT;
