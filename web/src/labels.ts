import type { AuditStatus, DraftStatus, Resolution } from './api'

// Names users understand (what happened), not rule codes.
export const ruleLabel: Record<string, string> = {
  PRICE_MISMATCH: 'Sözleşmeden yüksek fiyat',
  QTY_EXCEEDS_DELIVERY: 'Teslim alınandan fazla miktar',
  MATURITY_VIOLATION: 'Sözleşmeden kısa vade',
  DUPLICATE_SUSPECT: 'Mükerrer fatura şüphesi',
  WITHHOLDING_ERROR: 'Hatalı tevkifat',
  PRICE_CREEP: 'Sessiz fiyat artışı',
  UNCONTRACTED_ITEM: 'Sözleşmede olmayan kalem',
  UOM_MISMATCH: 'Birim uyuşmuyor',
  FX_UNVERIFIED: 'Kur doğrulanamadı',
  FX_RATE_DEVIATION: 'Faturadaki kur TCMB’den farklı',
}

export const statusLabel: Record<AuditStatus, string> = {
  RED: 'Fark var',
  YELLOW: 'Uyarı var',
  GREEN: 'Temiz',
  PENDING: 'Denetim bekliyor',
}

export const resolutionLabel: Record<Resolution, string> = {
  OPEN: 'Karar bekliyor',
  DISPUTED: 'İtiraz edildi',
  APPROVED_EXCEPTION: 'İstisna olarak onaylandı',
}

export const draftStatusLabel: Record<DraftStatus, string> = {
  EXTRACTING: 'Okunuyor',
  READY: 'İncelemeni bekliyor',
  FAILED: 'Okunamadı',
  APPROVED: 'Onaylandı, sözleşme aktif',
}
