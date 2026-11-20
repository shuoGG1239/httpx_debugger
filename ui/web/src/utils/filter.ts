import type { Session } from '@/types'

// 过滤表达式语法（完整布尔）：
//   - AND / OR 严格大写作为运算符（小写按普通关键词匹配）
//   - 支持括号分组：(GET OR POST) AND 200
//   - 每个关键词的智能匹配：纯数字→状态码精确；HTTP 方法词→method；其他→多字段包含
const HTTP_METHODS = ['get', 'post', 'put', 'patch', 'delete', 'head', 'options', 'connect', 'trace']

type FilterNode =
  | { op: 'and' | 'or'; children: FilterNode[] }
  | { kw: string }

/** 词法：按空白拆分，并把括号拆为独立 token（支持 `(GET`、`POST)` 无空格写法）。 */
function tokenize(q: string): string[] {
  const tokens: string[] = []
  for (const w of q.trim().split(/\s+/)) {
    if (!w) continue
    let buf = ''
    for (const ch of w) {
      if (ch === '(' || ch === ')') {
        if (buf) {
          tokens.push(buf)
          buf = ''
        }
        tokens.push(ch)
      } else {
        buf += ch
      }
    }
    if (buf) tokens.push(buf)
  }
  return tokens
}

/** 递归下降解析器：expr := or ; or := and (OR and)* ; and := atom (AND atom)* ; atom := kw | '(' expr ')' */
class FilterParser {
  private pos = 0
  constructor(private tokens: string[]) {}

  parse(): FilterNode | null {
    if (this.tokens.length === 0) return null
    return this.parseOr()
  }

  private parseOr(): FilterNode {
    let left = this.parseAnd()
    while (this.peek() === 'OR') {
      this.next()
      left = { op: 'or', children: [left, this.parseAnd()] }
    }
    return left
  }

  private parseAnd(): FilterNode {
    let left = this.parseAtom()
    while (true) {
      const t = this.peek()
      if (t === 'AND') {
        this.next()
        left = { op: 'and', children: [left, this.parseAtom()] }
      } else if (t !== undefined && t !== ')' && t !== 'OR') {
        // 隐式 AND：空格分隔的相邻条件（含小写 and/or 这类普通关键词）
        left = { op: 'and', children: [left, this.parseAtom()] }
      } else {
        break
      }
    }
    return left
  }

  private parseAtom(): FilterNode {
    const t = this.next()
    if (t === '(') {
      const node = this.parseOr()
      this.next() // 期望 ')'；不匹配时宽松忽略
      return node
    }
    // 意外 token（悬空 AND/OR、多余右括号、结尾）→ 空关键词，不匹配任何会话
    if (t === undefined || t === ')' || t === 'AND' || t === 'OR') {
      return { kw: '' }
    }
    return { kw: t }
  }

  private peek(): string | undefined {
    return this.tokens[this.pos]
  }

  private next(): string | undefined {
    return this.tokens[this.pos++]
  }
}

function tokenMatches(s: Session, t: string): boolean {
  // 纯数字 → 状态码
  if (/^\d+$/.test(t)) {
    return String(s.status) === t
  }
  // HTTP 方法词 → method
  if (HTTP_METHODS.includes(t)) {
    return s.method.toLowerCase() === t
  }
  // 其他关键词 → 多字段包含匹配
  const haystacks = [
    s.method,
    s.host,
    s.url,
    s.scheme,
    s.statusText,
    s.requestContentType,
    s.responseContentType,
    s.error ?? '',
  ]
  return haystacks.some((h) => h.toLowerCase().includes(t))
}

function evalNode(s: Session, node: FilterNode): boolean {
  if ('kw' in node) {
    if (!node.kw) return false
    return tokenMatches(s, node.kw.toLowerCase())
  }
  return node.op === 'and'
    ? node.children.every((c) => evalNode(s, c))
    : node.children.some((c) => evalNode(s, c))
}

export function sessionMatches(s: Session, q: string): boolean {
  const node = new FilterParser(tokenize(q)).parse()
  if (!node) return true
  return evalNode(s, node)
}
