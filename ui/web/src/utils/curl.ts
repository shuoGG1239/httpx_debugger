import type { Session } from '@/types'
import { bodyBytes, bytesToText, isBinary } from './bytes'

/** 把会话请求转成 curl 命令（用于复制/分享）。 */
export function sessionToCurl(s: Session): string {
  const lines = [`curl '${s.url}'`]
  if (s.method !== 'GET') lines.push(`  -X ${s.method}`)
  for (const h of s.requestHeaders) {
    if (/^content-length$/i.test(h.name)) continue
    lines.push(`  -H '${h.name}: ${h.value}'`)
  }
  const b = bodyBytes(s.requestBody)
  if (b.length > 0) {
    if (isBinary(b)) {
      lines.push(`  # body: ${b.length} bytes (binary, not included)`)
    } else {
      lines.push(`  --data-raw '${bytesToText(b)}'`)
    }
  }
  return lines.join(' \\\n')
}
