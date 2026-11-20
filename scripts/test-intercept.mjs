// 断点改包 + 重放 协议测试：
//   请求拦截（修改后继续 / 丢弃）→ 响应拦截（修改状态与体）→ 拦截超时自动放行 → 重放命令
// 前置：core 已在 127.0.0.1:8900 监听。用法: node scripts/test-intercept.mjs
import net from 'node:net'
import { execFile, spawn } from 'node:child_process'

const HOST = '127.0.0.1'
const PORT = 8900
const PROXY = 'http://127.0.0.1:8899'
const ECHO = 'http://127.0.0.1:8123'

const socket = net.createConnection({ host: HOST, port: PORT })
let buf = ''
const events = []
let sessionCount = 0
let nextId = 100
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
      const msg = JSON.parse(line)
      events.push(msg)
      if (msg.type === 'session') sessionCount++
    } catch {
      /* ignore */
    }
  }
})

const send = (obj) =>
  new Promise((resolve) => {
    const id = nextId++
    socket.write(JSON.stringify({ ...obj, id }) + '\n')
    const t0 = Date.now()
    const iv = setInterval(() => {
      const hit = events.find((e) => e.type === 'reply' && e.id === id)
      if (hit) {
        clearInterval(iv)
        resolve(hit)
      } else if (Date.now() - t0 > 10000) {
        clearInterval(iv)
        resolve({ ok: false, error: 'no reply' })
      }
    }, 20)
  })

const waitFor = (pred, label, timeoutMs = 12000) =>
  new Promise((resolve, reject) => {
    const t0 = Date.now()
    const iv = setInterval(() => {
      const idx = events.findIndex(pred)
      if (idx >= 0) {
        clearInterval(iv)
        resolve(events.splice(idx, 1)[0]) // 消费式取出，避免匹配到历史事件
      } else if (Date.now() - t0 > timeoutMs) {
        clearInterval(iv)
        reject(new Error('timeout waiting for ' + label))
      }
    }, 25)
  })

const curl = (args) =>
  new Promise((resolve) => {
    execFile('curl.exe', args, { timeout: 25000 }, (err, stdout, stderr) => {
      resolve({ code: err ? err.code : 0, stdout: (stdout || '').trim(), stderr: (stderr || '').trim() })
    })
  })

const b64 = (s) => Buffer.from(s, 'utf8').toString('base64')
const db64 = (s) => (s ? Buffer.from(s, 'base64').toString('utf8') : '')

function check(name, cond, detail) {
  if (cond) console.log(`  PASS  ${name}`)
  else {
    failures++
    console.log(`  FAIL  ${name}  ${detail ?? ''}`)
  }
}

const wait = (ms) => new Promise((r) => setTimeout(r, ms))

async function main() {
  await new Promise((r) => socket.once('connect', r))
  await wait(400)

  console.log('--- 环境准备 ---')
  let r = await send({ cmd: 'start', listen: '127.0.0.1:8899' })
  check('proxy start', r.ok === true)
  const echo = spawn(process.execPath, [new URL('./test-echo-server.mjs', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'), '8123'], { stdio: 'ignore' })
  await wait(800)

  console.log('--- 开启拦截（host 匹配 echo，方向 both）---')
  r = await send({ cmd: 'setIntercept', enabled: true, pattern: '127.0.0.1:8123', direction: 'both', timeoutMs: 4000 })
  check('setIntercept ok', r.ok === true)

  console.log('--- TEST A: 请求拦截并修改后继续 ---')
  {
    const curlP = curl(['-s', '-w', '\n%{http_code}', '-x', PROXY, '-X', 'POST', ECHO + '/echo', '-H', 'Content-Type: text/plain', '-H', 'X-Orig: 1', '--data-binary', 'orig-body'])
    const ev = await waitFor((e) => e.type === 'interceptRequest', 'interceptRequest A')
    check('收到请求拦截', !!ev, JSON.stringify(ev))
    check('拦截请求体正确', db64(ev.request.body) === 'orig-body', `got: ${db64(ev.request.body)}`)
    const reply = await send({
      cmd: 'interceptContinue',
      sessionId: ev.sessionId,
      request: {
        method: 'POST',
        url: ECHO + '/echo',
        headers: [
          { name: 'Content-Type', value: 'text/plain' },
          { name: 'X-Modified', value: 'yes' },
        ],
        body: b64('modified-body'),
      },
    })
    check('interceptContinue ok', reply.ok === true, reply.error)
    // direction=both 时响应也会被拦截，原样放行
    const evR = await waitFor((e) => e.type === 'interceptResponse', 'interceptResponse A')
    const replyR = await send({ cmd: 'interceptResponseContinue', sessionId: evR.sessionId })
    check('响应拦截原样放行 ok', replyR.ok === true, replyR.error)
    const out = await curlP
    const [bodyText, code] = out.stdout.split('\n')
    const echoJson = JSON.parse(bodyText)
    check('上游收到修改后的 body', echoJson.body === 'modified-body', echoJson.body)
    check('上游收到修改后的 header', echoJson.headers['x-modified'] === 'yes', JSON.stringify(echoJson.headers))
    check('响应 200', code === '200', code)
  }

  console.log('--- TEST B: 请求拦截并丢弃 ---')
  {
    const curlP = curl(['-s', '-o', 'NUL', '-w', '%{http_code}', '-x', PROXY, ECHO + '/drop'])
    const ev = await waitFor((e) => e.type === 'interceptRequest', 'interceptRequest B')
    check('收到请求拦截 B', !!ev)
    const reply = await send({ cmd: 'interceptDrop', sessionId: ev.sessionId })
    check('interceptDrop ok', reply.ok === true, reply.error)
    const code = await curlP
    check('客户端收到 403', code.stdout === '403', code.stdout)
  }

  console.log('--- TEST C: 响应拦截并修改状态/体 ---')
  {
    const curlP = curl(['-s', '-w', '\n%{http_code}|%{size_download}', '-x', PROXY, ECHO + '/resp'])
    const evReq = await waitFor((e) => e.type === 'interceptRequest', 'interceptRequest C')
    await send({ cmd: 'interceptContinue', sessionId: evReq.sessionId })
    const ev = await waitFor((e) => e.type === 'interceptResponse', 'interceptResponse C')
    check('收到响应拦截', !!ev, JSON.stringify(ev))
    const reply = await send({
      cmd: 'interceptResponseContinue',
      sessionId: ev.sessionId,
      response: {
        status: 201,
        statusText: 'Created',
        headers: [{ name: 'Content-Type', value: 'text/plain' }, { name: 'X-Mod', value: 'yes' }],
        body: b64('modified-response'),
      },
    })
    check('interceptResponseContinue ok', reply.ok === true, reply.error)
    const out = await curlP
    const [bodyText, meta] = out.stdout.split('\n')
    check('客户端收到 201', meta.startsWith('201'), meta)
    check('客户端收到修改后的 body', bodyText === 'modified-response', bodyText)
  }

  console.log('--- TEST D: 拦截超时自动放行 ---')
  {
    // 先把方向切到 request，避免响应也被拦截再等一轮
    r = await send({ cmd: 'setIntercept', enabled: true, pattern: '127.0.0.1:8123', direction: 'request', timeoutMs: 2000 })
    check('setIntercept direction=request', r.ok === true)
    const t0 = Date.now()
    const curlP = curl(['-s', '-o', 'NUL', '-w', '%{http_code}', '-x', PROXY, ECHO + '/timeout'])
    const ev = await waitFor((e) => e.type === 'interceptRequest', 'interceptRequest D')
    check('收到请求拦截 D', !!ev)
    const code = await curlP // 不裁决，等待超时放行
    const elapsed = Date.now() - t0
    check('超时后自动放行且成功', code.stdout === '200', `${code.stdout} elapsed=${elapsed}ms`)
    check('放行耗时 >= 超时值', elapsed >= 2000, `${elapsed}ms`)
  }

  console.log('--- TEST E: 重放命令 ---')
  {
    const before = sessionCount
    r = await send({
      cmd: 'replay',
      request: {
        method: 'GET',
        url: ECHO + '/replay',
        headers: [{ name: 'X-Replay', value: '1' }],
        body: '',
      },
    })
    check('replay ok', r.ok === true, r.error)
    const ev = await waitFor((e) => e.type === 'session' && e.session.url.includes('/replay'), 'replay session')
    check('重放产生新会话', !!ev)
    check('重放会话状态 200', ev.session.status === 200, String(ev.session.status))
    const respText = db64(ev.session.responseBody)
    check('重放响应头 X-Replay 生效', respText.includes('"x-replay":"1"'), respText.slice(0, 120))
    check('重放会话计数 +1', sessionCount === before + 1, `${before} -> ${sessionCount}`)
  }

  console.log('--- 收尾 ---')
  await send({ cmd: 'setIntercept', enabled: false, pattern: '', direction: 'both' })
  await send({ cmd: 'stop' })
  echo.kill()
  socket.end()

  console.log(failures === 0 ? '\nALL PASS' : `\n${failures} FAILURES`)
  process.exit(failures === 0 ? 0 : 1)
}

main().catch((e) => {
  console.error('FATAL:', e.message)
  process.exit(1)
})
