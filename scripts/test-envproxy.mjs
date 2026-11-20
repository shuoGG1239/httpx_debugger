// 验证「环境变量代理」方案：不显式 -x，仅靠 HTTP_PROXY/HTTPS_PROXY 环境变量，curl 的流量是否被 core 抓到。
// 前置：core 已在 127.0.0.1:8900 监听。用法: node scripts/test-envproxy.mjs
import net from 'node:net'
import { execFile } from 'node:child_process'

const socket = net.createConnection({ host: '127.0.0.1', port: 8900 })
let buf = ''
const sessions = []
let failures = 0

socket.on('error', (e) => console.error('[socket error]', e.message))
socket.on('data', (d) => {
  buf += d.toString('utf8')
  let i
  while ((i = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, i)
    buf = buf.slice(i + 1)
    if (!line.trim()) continue
    try {
      const m = JSON.parse(line)
      if (m.type === 'session') sessions.push(m.session)
    } catch {
      /* ignore */
    }
  }
})

const send = (obj) =>
  new Promise((resolve) => {
    socket.write(JSON.stringify({ ...obj, id: 1 }) + '\n')
    setTimeout(resolve, 300)
  })
const wait = (ms) => new Promise((r) => setTimeout(r, ms))

const run = (args, env) =>
  new Promise((resolve) => {
    execFile('curl.exe', args, { timeout: 20000, env: { ...process.env, ...env } }, (err, stdout, stderr) => {
      resolve({ code: err ? err.code : 0, stdout: (stdout || '').trim(), stderr: (stderr || '').trim() })
    })
  })

function check(name, cond, detail) {
  if (cond) console.log(`  PASS  ${name}`)
  else {
    failures++
    console.log(`  FAIL  ${name}  ${detail ?? ''}`)
  }
}

async function main() {
  await new Promise((r) => socket.once('connect', r))
  await wait(400)
  await send({ cmd: 'start', listen: '127.0.0.1:8899' })

  console.log('--- 无任何代理设置：curl 直连 ---')
  const r1 = await run(['-s', '-o', 'NUL', '-w', '%{http_code}', 'http://www.baidu.com/'], {})
  check(`curl 直连成功 (${r1.stdout})`, r1.stdout === '200')
  await wait(300)
  console.log(`  此时抓到会话数: ${sessions.length}`)

  console.log('--- 设置 HTTP_PROXY/HTTPS_PROXY 后：curl 不加 -x ---')
  const env = { HTTP_PROXY: 'http://127.0.0.1:8899', HTTPS_PROXY: 'http://127.0.0.1:8899' }
  const r2 = await run(['-s', '-o', 'NUL', '-w', '%{http_code}', 'http://www.baidu.com/'], env)
  check(`curl 经环境变量代理成功 (${r2.stdout})`, r2.stdout === '200')
  await wait(300)
  check(`流量被 core 抓到 (会话数 ${sessions.length})`, sessions.length === 1, JSON.stringify(sessions.map((s) => s.url)))

  await send({ cmd: 'stop' })
  socket.end()
  console.log(failures === 0 ? '\nALL PASS：环境变量方案有效' : `\n${failures} FAILURES`)
  process.exit(failures === 0 ? 0 : 1)
}

main().catch((e) => {
  console.error('FATAL:', e.message)
  process.exit(1)
})
