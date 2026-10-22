// Tab in the lists of /open and /workdir: what the omnibox should say next. The typed text is a path; the rows
// are what matches it. With several rows that agree on more letters than were typed, those letters are added;
// otherwise the highlighted row is taken (a folder is stepped into: its text ends with a slash).
export function completion(text, items, sel) {
  const dirPart = text.slice(0, text.lastIndexOf('/') + 1)
  const rows = items.filter((it) => !it.label.startsWith('·')) // "· this folder" is not a path
  if (!rows.length) return null
  const full = (it) => (it.dir ? it.query : dirPart + (it.meta && it.meta.endsWith('/') ? it.meta : '') + it.label)
  const all = rows.map(full)
  let common = all[0]
  for (const c of all) while (!c.toLowerCase().startsWith(common.toLowerCase())) common = common.slice(0, -1)
  if (all.length > 1 && common.length > text.length && common.toLowerCase().startsWith(text.toLowerCase())) return common
  const chosen = items[sel] && !items[sel].label.startsWith('·') ? items[sel] : rows[0]
  return full(chosen)
}
