export function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

export function bytesToText(bytes: Uint8Array): string {
  try {
    return new TextDecoder('utf-8', { fatal: false }).decode(bytes)
  } catch {
    return ''
  }
}

/** UTF-8 文本编码为 base64（与 Go []byte 的 JSON 编码一致）。 */
export function textToB64(text: string): string {
  const bytes = new TextEncoder().encode(text)
  let bin = ''
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i])
  return btoa(bin)
}

export function isBinary(bytes: Uint8Array): boolean {
  const n = Math.min(bytes.length, 1024)
  if (n === 0) return false
  let bad = 0
  for (let i = 0; i < n; i++) {
    const c = bytes[i]
    if (c === 0 || (c < 32 && c !== 9 && c !== 10 && c !== 13)) bad++
  }
  return bad / n > 0.2
}

export function hexDump(bytes: Uint8Array, maxBytes = 8192): string {
  const n = Math.min(bytes.length, maxBytes)
  const lines: string[] = []
  for (let off = 0; off < n; off += 16) {
    const hex: string[] = []
    const ascii: string[] = []
    for (let i = off; i < Math.min(off + 16, n); i++) {
      hex.push(bytes[i].toString(16).padStart(2, '0'))
      const c = bytes[i]
      ascii.push(c >= 32 && c < 127 ? String.fromCharCode(c) : '.')
    }
    lines.push(
      off.toString(16).padStart(8, '0') +
        '  ' +
        hex.join(' ').padEnd(47) +
        '  |' +
        ascii.join('') +
        '|',
    )
  }
  if (bytes.length > maxBytes) lines.push(`… 截断，共 ${bytes.length} 字节，仅显示前 ${maxBytes} 字节`)
  return lines.join('\n')
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(2)} MB`
}

export function formatTime(ms: number): string {
  const d = new Date(ms)
  const p = (x: number, l = 2) => String(x).padStart(l, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}.${p(d.getMilliseconds(), 3)}`
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

/** 从 body（base64）取出 Uint8Array；空字符串返回空数组。 */
export function bodyBytes(b64: string): Uint8Array {
  if (!b64) return new Uint8Array(0)
  return b64ToBytes(b64)
}
