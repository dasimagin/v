import YAML from 'yaml'

// Text in, a value out, for the tree view: JSON, or YAML (a file with several documents becomes a list of them).
export function parseData(text, kind) {
  try {
    if (kind === 'json') return { ok: true, value: JSON.parse(text) }
    const docs = YAML.parseAllDocuments(text)
    const bad = docs.find((d) => d.errors.length)
    if (bad) return { ok: false, error: bad.errors[0].message.split('\n')[0] }
    const values = docs.map((d) => d.toJS()).filter((v, i, all) => !(v === null && all.length > 1 && i === all.length - 1))
    return { ok: true, value: values.length === 1 ? values[0] : values }
  } catch (e) {
    return { ok: false, error: String(e.message ?? e).split('\n')[0] }
  }
}

export const kindOfValue = (v) => (v === null ? 'null' : Array.isArray(v) ? 'array' : typeof v)

// The entries of a container as [key, value] pairs.
export const entries = (v) => (Array.isArray(v) ? v.map((x, i) => [i, x]) : Object.entries(v))

// A short look at what a folded node holds: `{ name, version, … }` or `[ 3 ]`.
export function preview(v) {
  if (Array.isArray(v)) return v.length ? `[ ${v.length} ]` : '[ ]'
  const keys = Object.keys(v)
  if (!keys.length) return '{ }'
  const shown = keys.slice(0, 4).join(', ')
  return `{ ${shown}${keys.length > 4 ? ', …' : ''} }`
}
