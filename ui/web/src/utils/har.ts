import type { Session } from '@/types'
import { bodyBytes, bytesToText, isBinary } from './bytes'

/**
 * 按 HAR 1.2 规范把会话导出为 JSON 字符串。
 * 二进制响应体以 base64 放入 content.encoding。
 */
export function buildHar(sessions: Session[]): string {
  const entries = sessions
    .filter((s) => s.scheme === 'http' || s.scheme === 'https')
    .map((s) => {
      const startedDateTime = new Date(s.startedAt).toISOString()
      const reqBody = bodyBytes(s.requestBody)
      const respBody = bodyBytes(s.responseBody)
      return {
        startedDateTime,
        time: s.elapsedMs,
        request: {
          method: s.method,
          url: s.url,
          httpVersion: 'HTTP/1.1',
          cookies: [],
          headers: s.requestHeaders.map((h) => ({ name: h.name, value: h.value })),
          queryString: [],
          headersSize: -1,
          bodySize: reqBody.length,
          postData:
            reqBody.length > 0
              ? {
                  mimeType: s.requestContentType || 'application/octet-stream',
                  ...(isBinary(reqBody)
                    ? { encoding: 'base64', text: s.requestBody }
                    : { text: bytesToText(reqBody) }),
                }
              : undefined,
        },
        response: {
          status: s.status,
          statusText: s.statusText,
          httpVersion: 'HTTP/1.1',
          cookies: [],
          headers: s.responseHeaders.map((h) => ({ name: h.name, value: h.value })),
          content:
            respBody.length > 0
              ? {
                  size: respBody.length,
                  mimeType: s.responseContentType || 'application/octet-stream',
                  ...(isBinary(respBody)
                    ? { encoding: 'base64', text: s.responseBody }
                    : { text: bytesToText(respBody) }),
                }
              : { size: 0, mimeType: s.responseContentType || '' },
          redirectURL: '',
          headersSize: -1,
          bodySize: respBody.length,
        },
        cache: {},
        timings: { send: 0, wait: Math.round(s.elapsedMs / 2), receive: Math.round(s.elapsedMs / 2) },
      }
    })
  return JSON.stringify(
    {
      log: {
        version: '1.2',
        creator: { name: 'httpx-debugger', version: '0.1.0' },
        entries,
      },
    },
    null,
    2,
  )
}
