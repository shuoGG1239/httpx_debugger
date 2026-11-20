// httpx-core 控制协议冒烟测试：连接 8900 → start → 经代理抓明文 HTTP / HTTPS MITM → 系统代理开关 → stop
// 用法: node scripts/test-control.mjs
import net from 'node:net'
import { execFile } from 'node:child_process'

const HOST = '127.0.0.1'
const PORT = 8900
const PROXY = 'http://127.0.0.1:8899'

const socket = net.createConnection({ host: HOST, port: PORT })
let buf = ''
const events = []
const sessions = []

socket.on('error', (e) => console.error('[socket error]', e.message))
socket.on('close', (hadError) => console.log(`[socket closed] hadError=${hadError}`))

socket.on('data', (d) => {
  buf += d.toString('utf8')
  let idx
  while ((idx = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, idx)
    buf = buf.slice(idx + 1)
    if (!line.trim()) continue
    try {
      const msg = JSON.parse(line)
      events.push(msg)
      if (msg.type === 'session') sessions.push(msg.session)
    } catch {
      /* ignore */
    }
  }
})

const send = (obj) => socket.write(JSON.stringify(obj) + '\n')
const wait = (ms) => new Promise((r) => setTimeout(r, ms))

const run = (args) =>
  new Promise((resolve) => {
    execFile('curl.exe', args, { timeout: 20000 }, (err, stdout, stderr) => {
      resolve({
        code: err ? err.code : 0,
        stdout: (stdout || '').trim(),
        stderr: (stderr || '').trim(),
      })
    })
  })

async function main() {
  await new Promise((r) => socket.once('connect', r))
  await wait(400)

  send({ id: 1, cmd: 'start', listen: '127.0.0.1:8899' })
  await wait(400)

  console.log('--- 明文 HTTP ---')
  const r1 = await run(['-s', '-o', 'NUL', '-w', '%{http_code}', '-x', PROXY, 'http://www.baidu.com/'])
  console.log('plain status:', r1.code === 0 ? r1.stdout : 'ERR ' + r1.stderr)
  await wait(400)

  console.log('--- HTTPS MITM ---')
  const r2 = await run([
    '-s', '-k', '-o', 'NUL', '-w', '%{http_code}',
    '-H', 'Accept-Encoding: gzip',
    '-x', PROXY, 'https://www.demo.cn/',
  ])
  console.log('https status:', r2.code === 0 ? r2.stdout : 'ERR ' + r2.stderr)
  await wait(400)

  console.log('--- 系统代理 set/unset ---')
  send({ id: 3, cmd: 'setProxy', port: 8899 })
  await wait(800)
  send({ id: 4, cmd: 'unsetProxy' })
  await wait(800)

  send({ id: 2, cmd: 'stop' })
  await wait(400)
  socket.end()
  await wait(300)

  console.log(`sessions captured: ${sessions.length}`)
  for (const s of sessions) {
    console.log(
      `  [${s.scheme}] ${s.method} ${s.url} -> ${s.status} ${s.statusText} ` +
        `req=${s.requestBody?.length ?? 0}B resp=${s.responseBody?.length ?? 0}B ` +
        `ct=${s.responseContentType || '-'} trunc=${s.requestTruncated || s.responseTruncated} err=${s.error || '-'}`,
    )
  }
  const replies = events.filter((e) => e.type === 'reply')
  const statuses = events.filter((e) => e.type === 'status')
  console.log(`replies: ${JSON.stringify(replies)}`)
  console.log(`status events: ${statuses.length} -> last: ${JSON.stringify(statuses[statuses.length - 1])}`)
  process.exit(0)
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
