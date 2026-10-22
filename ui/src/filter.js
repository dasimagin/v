// The settings that match the search text.
export const visible = (opts, q) => {
  q = q.trim().toLowerCase()
  return q ? opts.filter((o) => (o.label + ' ' + o.desc).toLowerCase().includes(q)) : opts
}

// Whether the letters of q appear in s in order, ignoring case: a loose match.
export function fuzzy(q, s) {
  q = q.replace(/\s+/g, '').toLowerCase()
  s = s.toLowerCase()
  let at = 0
  for (const ch of q) {
    at = s.indexOf(ch, at)
    if (at < 0) return false
    at++
  }
  return true
}
