// 本地回显服务器：把收到的请求方法/URL/头/体回显为 JSON，用于验证拦截修改是否真正生效。
// 用法: node scripts/test-echo-server.mjs [port]
import http from 'node:http'

const port = Number(process.argv[2] || 8123)
const server = http.createServer((req, res) => {
  const chunks = []
  req.on('data', (c) => chunks.push(c))
  req.on('end', () => {
    const body = Buffer.concat(chunks).toString('utf8')
    const payload = { method: req.method, url: req.url, headers: req.headers, body }
    res.writeHead(200, { 'Content-Type': 'application/json', 'X-Echo-Server': '1' })
    res.end(JSON.stringify(payload))
  })
})
server.listen(port, '127.0.0.1', () => {
  console.log(`echo server listening on http://127.0.0.1:${port}`)
})
