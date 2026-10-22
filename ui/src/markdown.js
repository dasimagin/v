// A small Markdown reader: text in, a tree of blocks out. It builds data, never HTML, and the
// page draws it with text nodes only, so nothing in a file can run as a script.

const FENCE = /^\s*(```|~~~)\s*([\w+-]*)\s*$/
const HEADING = /^(#{1,6})\s+(.*?)\s*#*\s*$/
const RULE = /^\s{0,3}([-*_])(\s*\1){2,}\s*$/
const ITEM = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/
const ROW = /^\s*\|?.*\|.*$/
const SEP = /^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/

export function linesToText(lines) {
  return lines.map((l) => l.map((s) => s.t).join('')).join('\n')
}

export const isMarkdown = (path) => /\.(md|markdown|mdx)$/i.test(path ?? '')

const cells = (row) =>
  row
    .trim()
    .replace(/^\|/, '')
    .replace(/\|$/, '')
    .split('|')
    .map((c) => c.trim())

export function parse(text) {
  return blocks(text.replace(/\r\n?/g, '\n').split('\n'))
}

function blocks(lines) {
  const out = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    if (!line.trim()) {
      i++
      continue
    }
    let m
    if ((m = line.match(FENCE))) {
      const body = []
      i++
      while (i < lines.length && !lines[i].match(FENCE)) body.push(lines[i++])
      i++
      out.push({ t: 'code', lang: m[2], text: body.join('\n') })
    } else if ((m = line.match(HEADING))) {
      out.push({ t: 'h', level: m[1].length, inl: inline(m[2]) })
      i++
    } else if (RULE.test(line)) {
      out.push({ t: 'hr' })
      i++
    } else if (/^\s{0,3}>/.test(line)) {
      const body = []
      while (i < lines.length && /^\s{0,3}>/.test(lines[i])) body.push(lines[i++].replace(/^\s{0,3}>\s?/, ''))
      out.push({ t: 'quote', kids: blocks(body) })
    } else if (ITEM.test(line)) {
      const first = line.match(ITEM)
      const ordered = /\d/.test(first[2])
      const base = first[1].length
      const items = []
      while (i < lines.length) {
        const mm = lines[i].match(ITEM)
        if (mm && mm[1].length <= base + 1 && /\d/.test(mm[2]) === ordered) {
          items.push({ lines: [mm[3]] })
          i++
        } else if (items.length && lines[i].trim() && /^\s+/.test(lines[i]) && !mm) {
          items[items.length - 1].lines.push(lines[i].replace(/^\s{1,4}/, ''))
          i++
        } else if (items.length && mm && mm[1].length > base + 1) {
          items[items.length - 1].lines.push(lines[i].slice(Math.min(base + 2, mm[1].length)))
          i++
        } else break
      }
      out.push({ t: 'list', ordered, items: items.map((it) => ({ kids: blocks(it.lines).map((b) => (b.t === 'p' ? { ...b } : b)) })) })
    } else if (ROW.test(line) && i + 1 < lines.length && SEP.test(lines[i + 1]) && line.includes('|')) {
      const head = cells(line)
      i += 2
      const rows = []
      while (i < lines.length && lines[i].trim() && lines[i].includes('|')) rows.push(cells(lines[i++]))
      out.push({ t: 'table', head: head.map(inline), rows: rows.map((r) => r.map(inline)) })
    } else {
      const para = []
      while (i < lines.length && lines[i].trim() && !FENCE.test(lines[i]) && !HEADING.test(lines[i]) && !/^\s{0,3}>/.test(lines[i]) && !RULE.test(lines[i]) && !(para.length && ITEM.test(lines[i]))) {
        para.push(lines[i++].trim())
      }
      out.push({ t: 'p', inl: inline(para.join(' ')) })
    }
  }
  return out
}

// Inline spans: `code`, **strong**, *emphasis*, [text](link), ![alt](image).
export function inline(s) {
  const out = []
  let text = ''
  const flush = () => {
    if (text) out.push({ t: 'text', text })
    text = ''
  }
  for (let i = 0; i < s.length; ) {
    const c = s[i]
    let end
    if (c === '\\' && i + 1 < s.length) {
      text += s[i + 1]
      i += 2
    } else if (c === '`' && (end = s.indexOf('`', i + 1)) > i) {
      flush()
      out.push({ t: 'code', text: s.slice(i + 1, end) })
      i = end + 1
    } else if ((s.startsWith('**', i) || s.startsWith('__', i)) && (end = s.indexOf(s.slice(i, i + 2), i + 2)) > i + 2) {
      flush()
      out.push({ t: 'strong', kids: inline(s.slice(i + 2, end)) })
      i = end + 2
    } else if ((c === '*' || (c === '_' && !/\w/.test(s[i - 1] ?? ' '))) && (end = s.indexOf(c, i + 1)) > i + 1 && s[i + 1] !== ' ') {
      flush()
      out.push({ t: 'em', kids: inline(s.slice(i + 1, end)) })
      i = end + 1
    } else if ((c === '[' || (c === '!' && s[i + 1] === '['))) {
      const img = c === '!'
      const open = img ? i + 1 : i
      const close = s.indexOf(']', open)
      const paren = close > 0 && s[close + 1] === '(' ? s.indexOf(')', close + 2) : -1
      if (close > 0 && paren > 0) {
        flush()
        const label = s.slice(open + 1, close)
        const href = s.slice(close + 2, paren).trim().split(/\s+/)[0]
        out.push(img ? { t: 'image', alt: label, src: href } : { t: 'link', href, kids: inline(label) })
        i = paren + 1
      } else {
        text += c
        i++
      }
    } else {
      text += c
      i++
    }
  }
  flush()
  return out
}
