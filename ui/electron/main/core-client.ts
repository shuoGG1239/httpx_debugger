import net from 'node:net'

/**
 * CoreClient：与 httpx-core 的 8900 控制端口通信。
 * - JSON 行协议；断线自动重连（core 启动需要时间）。
 * - 未连接时发送的命令会排队，连接后补发。
 */
export class CoreClient {
  private socket: net.Socket | null = null
  private buf = ''
  private reconnectTimer: NodeJS.Timeout | null = null
  private stopped = false
  private queue: unknown[] = []
  private connState = false

  constructor(
    private port: number,
    private onMessage: (msg: unknown) => void,
    private onLifecycle: (state: { connected: boolean; error?: string }) => void,
  ) {}

  isConnected(): boolean {
    return this.connState
  }

  start(): void {
    this.stopped = false
    this.connect()
  }

  stop(): void {
    this.stopped = true
    this.connState = false
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    this.socket?.destroy()
    this.socket = null
  }

  send(msg: unknown): void {
    if (this.socket && this.socket.readyState === 'open') {
      this.rawSend(msg)
    } else {
      this.queue.push(msg)
    }
  }

  private connect(): void {
    if (this.stopped) return
    const s = net.createConnection({ host: '127.0.0.1', port: this.port })
    this.socket = s

    s.on('connect', () => {
      this.connState = true
      this.onLifecycle({ connected: true })
      while (this.queue.length > 0) {
        const msg = this.queue.shift()
        if (msg !== undefined) this.rawSend(msg)
      }
    })

    s.on('data', (d) => {
      this.buf += d.toString('utf8')
      let idx: number
      while ((idx = this.buf.indexOf('\n')) >= 0) {
        const line = this.buf.slice(0, idx)
        this.buf = this.buf.slice(idx + 1)
        if (!line.trim()) continue
        try {
          this.onMessage(JSON.parse(line))
        } catch {
          // 忽略解析失败的行
        }
      }
    })

    s.on('error', (e) => {
      this.onLifecycle({ connected: false, error: e.message })
    })

    s.on('close', () => {
      this.socket = null
      this.connState = false
      this.onLifecycle({ connected: false })
      if (!this.stopped) {
        this.reconnectTimer = setTimeout(() => this.connect(), 1000)
      }
    })
  }

  private rawSend(msg: unknown): void {
    if (this.socket && this.socket.readyState === 'open') {
      this.socket.write(JSON.stringify(msg) + '\n')
    }
  }
}
