import { defineStore } from 'pinia'
import type {
  CoreMessage,
  InterceptEntry,
  Lifecycle,
  ModifiedRequest,
  ModifiedResponse,
  Session,
  Status,
} from '@/types'
import { sessionMatches } from '@/utils/filter'

const MAX_SESSIONS = 5000

export const useCaptureStore = defineStore('capture', {
  state: () => ({
    connected: false,
    lifecycle: null as Lifecycle | null,
    status: null as Status | null,
    sessions: [] as Session[],
    selectedId: null as string | null,
    filter: '',
    autoScroll: true,
    interceptQueue: [] as InterceptEntry[],
    cmdSeq: 0,
    pendingReply: null as { id: number; kind: 'installCA' } | null,
    notice: null as { kind: 'ok' | 'err'; text: string } | null,
  }),

  getters: {
    selected(state): Session | null {
      return state.sessions.find((s) => s.id === state.selectedId) ?? null
    },
    filtered(state): Session[] {
      const q = state.filter.trim()
      if (!q) return state.sessions
      return state.sessions.filter((s) => sessionMatches(s, q))
    },
    sessionCount(state): number {
      return state.sessions.length
    },
    interceptEnabled(state): boolean {
      return !!state.status?.interceptEnabled
    },
    interceptPattern(state): string {
      return state.status?.interceptPattern ?? ''
    },
    interceptDirection(state): string {
      return state.status?.interceptDirection ?? 'both'
    },
    pendingIntercepts(state): number {
      return state.interceptQueue.length
    },
    redirectEnabled(state): boolean {
      return !!state.status?.redirectEnabled
    },
  },

  actions: {
    handleMessage(msg: CoreMessage): void {
      switch (msg.type) {
        case 'status':
          this.status = msg.status
          break
        case 'session': {
          this.sessions.push(msg.session)
          if (this.sessions.length > MAX_SESSIONS) {
            this.sessions.splice(0, this.sessions.length - MAX_SESSIONS)
          }
          break
        }
        case 'interceptRequest':
          this.interceptQueue.push({
            id: msg.sessionId,
            kind: 'request',
            ...msg.request,
          })
          break
        case 'interceptResponse':
          this.interceptQueue.push({
            id: msg.sessionId,
            kind: 'response',
            ...msg.response,
          })
          break
        case 'reply':
          if (this.pendingReply && msg.id === this.pendingReply.id) {
            this.pendingReply = null
            this.showNotice(
              msg.ok ? '证书已安装 ✓（本次及后续启动均免装）' : `证书安装失败：${msg.error ?? '未知错误'}`,
              msg.ok ? 'ok' : 'err',
            )
          }
          break
      }
    },
    showNotice(text: string, kind: 'ok' | 'err'): void {
      this.notice = { text, kind }
      setTimeout(() => {
        if (this.notice?.text === text) this.notice = null
      }, 6000)
    },
    setLifecycle(state: Lifecycle): void {
      this.lifecycle = state
      this.connected = state.connected
      if (!state.connected) this.status = null
    },
    setFilter(q: string): void {
      this.filter = q
    },
    select(id: string | null): void {
      this.selectedId = id
    },
    clearSessions(): void {
      window.api.sendCore({ cmd: 'clear' })
      this.sessions = []
      this.selectedId = null
    },
    toggleCapture(): void {
      // 开始捕获自动接管系统代理，停止捕获自动还原（core 处理）
      if (this.status?.capturing) {
        window.api.sendCore({ cmd: 'stop' })
      } else {
        window.api.sendCore({ cmd: 'start', listen: '127.0.0.1:8899' })
      }
    },
    installCA(): void {
      const id = ++this.cmdSeq
      this.pendingReply = { id, kind: 'installCA' }
      window.api.sendCore({ id, cmd: 'installCA' })
    },
    applyIntercept(config: { enabled?: boolean; pattern?: string; direction?: string }): void {
      const enabled = config.enabled ?? this.interceptEnabled
      const pattern = config.pattern ?? this.interceptPattern
      const direction = config.direction ?? this.interceptDirection
      // 乐观更新 UI，core 确认后 status 事件会同步
      if (this.status) {
        this.status = { ...this.status, interceptEnabled: enabled, interceptPattern: pattern, interceptDirection: direction }
      }
      window.api.sendCore({
        cmd: 'setIntercept',
        enabled,
        pattern,
        direction,
        timeoutMs: 60_000,
      })
    },
    toggleRedirect(): void {
      if (this.status?.redirectEnabled) {
        window.api.sendCore({ cmd: 'redirectStop' })
      } else {
        window.api.sendCore({ cmd: 'redirectStart' })
      }
    },
    resolveIntercept(id: string, action: 'continue' | 'drop', payload?: ModifiedRequest | ModifiedResponse): void {
      const entry = this.interceptQueue[0]
      if (!entry || entry.id !== id) return
      if (action === 'continue') {
        if (entry.kind === 'request') {
          window.api.sendCore({ cmd: 'interceptContinue', sessionId: id, request: payload as ModifiedRequest })
        } else {
          window.api.sendCore({ cmd: 'interceptResponseContinue', sessionId: id, response: payload as ModifiedResponse })
        }
      } else {
        window.api.sendCore({ cmd: 'interceptDrop', sessionId: id })
      }
      this.interceptQueue.shift()
    },
    replay(session: Session): void {
      window.api.sendCore({
        cmd: 'replay',
        request: {
          method: session.method,
          url: session.url,
          headers: session.requestHeaders,
          body: session.requestBody,
        },
      })
    },
  },
})
