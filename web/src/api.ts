// Thin client for the Go API under /api/v1. The session is an HttpOnly cookie; this code never sees the token.

export type AuditStatus = 'GREEN' | 'YELLOW' | 'RED' | 'PENDING'
export type Resolution = 'OPEN' | 'DISPUTED' | 'APPROVED_EXCEPTION'

export interface InvoiceRow {
  id: string
  vendor_id: string
  vendor_name: string
  invoice_no: string
  issue_date: string
  due_date: string | null
  currency: string
  payable_minor: number
  audit_status: AuditStatus
  open_findings: number
}

export interface ContractItemRow {
  id: string
  contract_no: string
  item_code: string
  name: string
  unit_price: string
  currency: string
  uom: string
  source_page: number
  source_clause_text: string
}

export interface DetailLine {
  id: string
  line_id: string
  item_code: string
  barcode: string
  name: string
  uom: string
  qty: string
  unit_price: string
  allowance_minor: number
  line_extension_minor: number
  contract_item: ContractItemRow | null
  despatch_lines: { despatch_no: string; line_id: string; allocated_qty: string }[] | null
}

export interface Suggestion {
  contract_item_id: string
  item_name: string
  similarity: string
}

export interface Finding {
  id: string
  line_id: string | null
  severity: 'CRITICAL' | 'WARNING' | 'INFO'
  rule_code: string
  expected: string
  actual: string
  financial_impact_minor: number
  impact_currency: string | null
  evidence: Record<string, unknown> & { suggestions?: Suggestion[] }
  resolution_status: Resolution
  resolution_note: string | null
}

export interface InvoiceDetail extends InvoiceRow {
  lines: DetailLine[]
  findings: Finding[]
}

export interface CurrencySummary {
  currency: string
  caught_minor: number
  disputed_minor: number
  open_count: number
  disputed_count: number
}

export interface Summary {
  by_currency: CurrencySummary[]
  invoices: Record<AuditStatus, number>
}

export interface Me {
  email: string
  tenant_name: string
}

export type DraftStatus = 'EXTRACTING' | 'READY' | 'FAILED' | 'APPROVED'

export interface DraftSummary {
  id: string
  status: DraftStatus
  contract_no: string
  vendor_name: string
  error_code?: string
  contract_id?: string
  created_at: string
}

export interface DraftItem {
  item_code?: string
  barcode?: string
  name: string
  unit_price: string | null
  currency: string
  uom: string
  withholding_code?: string
  requires_delivery?: boolean
  source_page?: number
  source_clause_text?: string
}

export interface ContractDraft {
  contract_no: string | null
  vendor_vkn: string | null
  vendor_name: string | null
  valid_from: string | null
  valid_until: string | null
  payment_term_days: number | null
  currency: string | null
  qty_tolerance_pct: string | null
  payment_term_source_page?: number | null
  payment_term_clause_text?: string | null
  items: DraftItem[]
}

export interface DraftWarning {
  code: string
  field?: string | null
  page?: number | null
  message?: string
}

export interface Draft {
  id: string
  status: DraftStatus
  draft: ContractDraft | null
  warnings: DraftWarning[]
  error_code?: string
  contract_id?: string
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message)
  }
}

let onUnauthorized: () => void = () => {}
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

async function request<T>(method: string, path: string, body?: BodyInit, contentType?: string): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (contentType) headers['Content-Type'] = contentType
  let res: Response
  try {
    res = await fetch(`/api/v1${path}`, { method, body, headers, credentials: 'same-origin' })
  } catch {
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı. Bağlantını kontrol edip tekrar dene.')
  }
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    if (res.status === 401 && path !== '/auth/login') onUnauthorized()
    throw new ApiError(res.status, data.code ?? 'UNKNOWN', data.error ?? `HTTP ${res.status}`)
  }
  return data as T
}

const json = (v: unknown) => JSON.stringify(v)
const id = encodeURIComponent

export const api = {
  login: (email: string, password: string) =>
    request<Me>('POST', '/auth/login', json({ email, password }), 'application/json'),
  logout: () => request<void>('POST', '/auth/logout'),
  me: () => request<Me>('GET', '/auth/me'),

  invoices: (status?: AuditStatus) =>
    request<{ items: InvoiceRow[]; total: number }>('GET', `/invoices?limit=200${status ? `&status=${status}` : ''}`),
  invoice: (invoiceId: string) => request<InvoiceDetail>('GET', `/invoices/${id(invoiceId)}`),
  reaudit: (invoiceId: string) => request<{ audit_status: AuditStatus }>('POST', `/invoices/${id(invoiceId)}/audit`),
  resolve: (findingId: string, resolution_status: Resolution, note: string) =>
    request<{ id: string; resolution_status: Resolution }>('PATCH', `/findings/${id(findingId)}`,
      json({ resolution_status, note }), 'application/json'),
  matchLine: (lineId: string, contractItemId: string) =>
    request<{ invoice_id: string; audit_status: AuditStatus }>('POST', `/invoice-lines/${id(lineId)}/match`,
      json({ contract_item_id: contractItemId }), 'application/json'),
  summary: () => request<Summary>('GET', '/summary'),

  uploadXML: (kind: 'invoices' | 'despatches' | 'receipts', xml: Blob) =>
    request<{ id?: string; audit_status?: AuditStatus; reaudited?: string[] }>('POST', `/${kind}`, xml, 'application/xml'),
  uploadContractPDF: (pdf: File) => {
    const form = new FormData()
    form.append('file', pdf)
    return request<Draft>('POST', '/contract-drafts', form)
  },
  drafts: () => request<{ items: DraftSummary[] }>('GET', '/contract-drafts'),
  draft: (draftId: string) => request<Draft>('GET', `/contract-drafts/${id(draftId)}`),
  reextract: (draftId: string) => request<Draft>('POST', `/contract-drafts/${id(draftId)}/extract`),
  approveDraft: (draftId: string, doc: ContractDraft) =>
    request<{ id: string }>('POST', '/contracts', json({ ...doc, draft_id: draftId }), 'application/json'),
}
