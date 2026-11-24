// Go API'si (/api/v1) için ince bir istemci. Oturum HttpOnly bir çerezde duruyor; bu kod token'ı hiç görmez.

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
  reaudit_pending: boolean
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
  decisions: Decision[] // en eskiden başlayarak; yeniden denetimden sonra da kalır
}

export interface Decision {
  resolution_status: Resolution
  note: string | null
  decided_by_email: string | null // dev token'larda null
  decided_at: string
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

export interface Dashboard {
  by_currency: CurrencySummary[]
  invoices: Record<AuditStatus, number>
  weekly: { week: string; currency: string; caught_minor: number }[]
  by_rule: { rule_code: string; currency: string; impact_minor: number; count: number }[]
  by_vendor: { vendor_id: string; vendor_name: string; currency: string; impact_minor: number; count: number }[]
  open_top: {
    finding_id: string
    invoice_id: string
    invoice_no: string
    vendor_name: string
    issue_date: string
    rule_code: string
    line_id: string | null
    impact_minor: number
    currency: string
  }[]
}

// GET /invoices parametreleri; arama, filtre, sıralama ve sayfalamayı sunucu yapar.
export interface InvoiceQuery {
  q?: string
  status?: AuditStatus
  vendor_id?: string
  from?: string // YYYY-MM-DD, uçlar dahil
  to?: string
  open?: boolean // yalnızca karar bekleyen bulgusu olan faturalar
  sort?: 'issue_date' | 'invoice_no' | 'vendor' | 'payable' | 'status' | 'open_findings'
  dir?: 'asc' | 'desc'
  limit?: number
  offset?: number
}

export interface InvoiceList {
  items: InvoiceRow[]
  total: number
  counts: Partial<Record<AuditStatus, number>> // duruma göre eşleşme sayıları, durum filtresi dikkate alınmadan
}

export interface FindingListRow {
  id: string
  invoice_id: string
  invoice_no: string
  vendor_name: string
  issue_date: string
  currency: string // faturanınki
  line_id: string | null
  item_name: string | null
  severity: Finding['severity']
  rule_code: string
  expected: string
  actual: string
  financial_impact_minor: number
  impact_currency: string | null
  resolution_status: Resolution
  resolution_note: string | null
}

export interface FindingQuery {
  resolution?: Resolution
  rule?: string
  vendor_id?: string
  limit?: number
  offset?: number
}

export interface FindingList {
  items: FindingListRow[] // etkisi en büyük olan önce
  total: number
  counts: Record<string, number> // kural koduna göre eşleşme sayıları, kural filtresi dikkate alınmadan
}

export interface AliasRow {
  id: string
  vendor_id: string
  vendor_name: string
  name_key: string // faturadaki kalem adı, normalleştirilmiş: küçük harf, Türkçe harfler sadeleştirilmiş
  uom: string
  contract_item: ContractItemRow
  created_at: string
  confirmed_by_email: string | null // dev token'la ya da kayıt tutulmaya başlamadan önce onaylandıysa null
}

// Kaldırılmış bir eşleşmenin kaydı: neydi, kim, ne zaman kaldırdı.
export interface RemovedAlias {
  vendor_id: string
  vendor_name: string
  name_key: string
  uom: string
  contract_no: string
  item_name: string
  removed_by_email: string | null
  removed_at: string
}

export interface VendorRow {
  id: string
  name: string
  vkn: string
  contract_no: string
  invoice_count: number
  open_findings: number
  caught: { currency: string; caught_minor: number }[]
}

export interface ContractRow {
  id: string
  contract_no: string
  status: 'ACTIVE' | 'INACTIVE'
  valid_from: string
  valid_until: string
  payment_term_days: number
  currency: string
  qty_tolerance_pct: string
  items: ContractItemRow[]
}

export interface VendorDetail extends VendorRow {
  contracts: ContractRow[]
  price_history: Record<string, { invoice_id: string; invoice_no: string; issue_date: string; net_unit_price: string; currency: string; uom: string }[]>
}

export interface Me {
  email: string
  tenant_name: string
}

export interface UserRow {
  id: string
  email: string
  created_at: string
}

export interface InviteRow extends UserRow {
  expires_at: string
}

export interface LinkInfo {
  kind: 'INVITE' | 'RESET'
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

// send, yanıt başarılıysa onu döner, değilse ApiError fırlatır.
async function send(method: string, path: string, body?: BodyInit, contentType?: string): Promise<Response> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (contentType) headers['Content-Type'] = contentType
  let res: Response
  try {
    res = await fetch(`/api/v1${path}`, { method, body, headers, credentials: 'same-origin' })
  } catch {
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı. Bağlantını kontrol edip tekrar dene.')
  }
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    // /auth/me giriş yapmamış herkese 401 döner: nereye gideceklerine oturum süresi bitti işleyicisi değil, router
    // karar verir
    if (res.status === 401 && path !== '/auth/login' && path !== '/auth/me') onUnauthorized()
    throw new ApiError(res.status, data.code ?? 'UNKNOWN', data.error ?? `HTTP ${res.status}`)
  }
  return res
}

async function request<T>(method: string, path: string, body?: BodyInit, contentType?: string): Promise<T> {
  const res = await send(method, path, body, contentType)
  if (res.status === 204) return undefined as T
  return (await res.json().catch(() => ({}))) as T
}

const json = (v: unknown) => JSON.stringify(v)
const id = encodeURIComponent

export const api = {
  login: (email: string, password: string) =>
    request<Me>('POST', '/auth/login', json({ email, password }), 'application/json'),
  signup: (b: { company: string; vkn: string; email: string; password: string }) =>
    request<Me>('POST', '/auth/signup', json(b), 'application/json'),
  logout: () => request<void>('POST', '/auth/logout'),
  me: () => request<Me>('GET', '/auth/me'),
  forgot: (email: string) => request<void>('POST', '/auth/forgot', json({ email }), 'application/json'),
  linkInfo: (token: string) => request<LinkInfo>('POST', '/auth/link', json({ token }), 'application/json'),
  redeem: (token: string, password: string) =>
    request<Me>('POST', '/auth/redeem', json({ token, password }), 'application/json'),
  users: () => request<{ users: UserRow[]; invites: InviteRow[] }>('GET', '/users'),
  // bağlantı yalnızca o adrese e-postayla gider; yanıtta asla yer almaz
  invite: (email: string) => request<InviteRow>('POST', '/invites', json({ email }), 'application/json'),
  revokeInvite: (inviteId: string) => request<void>('DELETE', `/invites/${id(inviteId)}`),

  invoices: (p: InvoiceQuery = {}) => {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== '' && v !== false) qs.set(k, String(v))
    return request<InvoiceList>('GET', `/invoices?${qs}`)
  },
  findings: (p: FindingQuery = {}) => {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== '') qs.set(k, String(v))
    return request<FindingList>('GET', `/findings?${qs}`)
  },
  dashboard: (from?: string) => request<Dashboard>('GET', `/dashboard${from ? `?from=${from}` : ''}`),
  vendors: () => request<{ items: VendorRow[] }>('GET', '/vendors'),
  vendor: (vendorId: string) => request<VendorDetail>('GET', `/vendors/${id(vendorId)}`),
  invoice: (invoiceId: string) => request<InvoiceDetail>('GET', `/invoices/${id(invoiceId)}`),
  reaudit: (invoiceId: string) => request<{ audit_status: AuditStatus }>('POST', `/invoices/${id(invoiceId)}/audit`),
  resolve: (findingId: string, resolution_status: Resolution, note: string) =>
    request<{ id: string; resolution_status: Resolution }>('PATCH', `/findings/${id(findingId)}`,
      json({ resolution_status, note }), 'application/json'),
  decideMany: (findingIds: string[], resolution_status: Resolution, note: string) =>
    request<{ updated: number }>('POST', '/findings/decisions',
      json({ finding_ids: findingIds, resolution_status, note }), 'application/json'),
  aliases: (vendorId?: string) => request<{ items: AliasRow[]; removed: RemovedAlias[] }>('GET', `/aliases${vendorId ? `?vendor_id=${id(vendorId)}` : ''}`),
  deleteAlias: (aliasId: string) => request<{ reaudited: string[] }>('DELETE', `/aliases/${id(aliasId)}`),
  matchLine: (lineId: string, contractItemId: string) =>
    request<{ invoice_id: string; audit_status: AuditStatus }>('POST', `/invoice-lines/${id(lineId)}/match`,
      json({ contract_item_id: contractItemId }), 'application/json'),

  uploadXML: (kind: 'invoices' | 'despatches' | 'receipts', xml: Blob) =>
    request<{ id?: string; audit_status?: AuditStatus; reaudited?: string[] }>('POST', `/${kind}`, xml, 'application/xml'),
  uploadContractPDF: (pdf: File) => {
    const form = new FormData()
    form.append('file', pdf)
    return request<Draft>('POST', '/contract-drafts', form)
  },
  contractPDF: async (contractNo: string) =>
    new Uint8Array(await (await send('GET', `/contracts/pdf?contract_no=${id(contractNo)}`)).arrayBuffer()),
  drafts: () => request<{ items: DraftSummary[] }>('GET', '/contract-drafts'),
  draft: (draftId: string) => request<Draft>('GET', `/contract-drafts/${id(draftId)}`),
  reextract: (draftId: string) => request<Draft>('POST', `/contract-drafts/${id(draftId)}/extract`),
  approveDraft: (draftId: string, doc: ContractDraft) =>
    request<{ id: string }>('POST', '/contracts', json({ ...doc, draft_id: draftId }), 'application/json'),
}
