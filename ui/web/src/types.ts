export interface Header {
  name: string
  value: string
}

export interface Session {
  id: string
  startedAt: number // unix 毫秒
  elapsedMs: number
  scheme: string // http / https / tunnel
  method: string
  host: string
  url: string
  status: number
  statusText: string
  error?: string
  clientAddr: string
  requestHeaders: Header[]
  requestBody: string // base64
  requestTruncated: boolean
  requestContentType: string
  responseHeaders: Header[]
  responseBody: string // base64
  responseTruncated: boolean
  responseContentType: string
}

export interface Status {
  capturing: boolean
  listen: string
  proxySet: boolean
  dataDir: string
  caInstalled: boolean
  maxBody: number
  certPath: string
  keyPath: string
  interceptEnabled: boolean
  interceptPattern: string
  interceptDirection: string
  redirectEnabled: boolean
  redirectPort: number
  redirectError?: string
}

export interface ModifiedRequest {
  method: string
  url: string
  headers: Header[]
  body: string // base64
}

export interface ModifiedResponse {
  status: number
  statusText: string
  headers: Header[]
  body: string // base64
}

/** 待处理的拦截项（由 core 的 interceptRequest/interceptResponse 事件产生） */
export type InterceptEntry =
  | { id: string; kind: 'request'; method: string; url: string; headers: Header[]; body: string }
  | { id: string; kind: 'response'; status: number; statusText: string; headers: Header[]; body: string }

export type CoreMessage =
  | { type: 'status'; status: Status }
  | { type: 'session'; session: Session }
  | { type: 'reply'; id: number; ok: boolean; error?: string }
  | { type: 'interceptRequest'; sessionId: string; request: ModifiedRequest }
  | { type: 'interceptResponse'; sessionId: string; response: ModifiedResponse }

export interface Lifecycle {
  connected: boolean
  binary: string | null
  error: string | null
}

export interface RendererApi {
  sendCore: (msg: unknown) => void
  onCoreEvent: (cb: (msg: unknown) => void) => () => void
  onCoreLifecycle: (cb: (state: unknown) => void) => () => void
  getLifecycle: () => Promise<{ connected: boolean; binary: string | null; error: string | null }>
  saveHar: (json: string, defaultName: string) => Promise<{
    ok: boolean
    canceled?: boolean
    path?: string
    error?: string
  }>
}
