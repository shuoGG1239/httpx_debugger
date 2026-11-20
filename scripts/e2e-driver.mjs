// 应用级 E2E 驱动：连接应用自带的 core（8900），开启拦截，等待外部 curl 命中后自动放行。
// 用法: node scripts/e2e-driver.mjs
import net from 'node:net'

const socket = net.createConnection({ host: '127.0.0.1', port: 8900 })
let buf = ''
let nextId = 1
let started = false

socket.on('error', (e) => {
  console.error('[driver error]', e.message)
  process.exit(2)
})
socket.on('close', () => console.log('[driver] socket closed'))

socket.on('data', (d) => {
  buf += d.toString('utf8')
  let i
  while ((i = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, i)
    buf = buf.slice(i + 1)
    if (!line.trim()) continue
    let m
    try {
      m = JSON.parse(line)
    } catch {
      continue
    }
    if (m.type === 'reply') {
      console.log(`[driver] reply id=${m.id} ok=${m.ok} err=${m.error ?? '-'}`)
    } else if (m.type === 'status') {
      console.log(
        `[driver] status capturing=${m.status.capturing} intercept=${m.status.interceptEnabled} pattern=${m.status.interceptPattern}`,
      )
    } else if (m.type === 'interceptRequest') {
      console.log(`[driver] interceptRequest ${m.sessionId} ${m.request.method} ${m.request.url}`)
      socket.write(
        JSON.stringify({
          id: nextId++,
          cmd: 'interceptContinue',
          sessionId: m.sessionId,
          request: { method: m.request.method, url: m.request.url, headers: m.request.headers, body: m.request.body },
        }) + '\n',
      )
      console.log('[driver] auto-continued, ready')
    } else if (m.type === 'interceptResponse') {
      console.log(`[driver] interceptResponse ${m.sessionId} ${m.response.status}`)
      socket.write(
        JSON.stringify({
          id: nextId++,
          cmd: 'interceptResponseContinue',
          sessionId: m.sessionId,
        }) + '\n',
      )
      console.log('[driver] auto-continued response')
    }
  }
})

function send(cmd) {
  const id = nextId++
  socket.write(JSON.stringify({ id, ...cmd }) + '\n')
}

socket.on('connect', () => {
  console.log('[driver] connected')
  send({ cmd: 'start', listen: '127.0.0.1:8899' })
  setTimeout(() => {
    send({ cmd: 'setIntercept', enabled: true, pattern: '127.0.0.1:8123', direction: 'both', timeoutMs: 8000 })
  }, 500)
  setTimeout(() => {
    if (!started) {
      started = true
      console.log('[driver] setup sent, waiting for intercepts...')
    }
  }, 1200)
})
