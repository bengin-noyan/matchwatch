-- Bir irsaliye, irsaliye yanıtı ya da kalem eşleşmesi değişince ilgili faturalar yeniden denetlenir. İşaret, değişiklikle
-- aynı transaction'da konur ve başarılı bir denetim onu kaldırır; kalırsa RetryPending faturayı toparlar.
ALTER TABLE invoices ADD COLUMN reaudit_since TIMESTAMPTZ;
