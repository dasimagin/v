// What a file shows as: Markdown rendered or as source, JSON and YAML as a tree or as text, an SVG drawn or as
// code, a picture drawn or as its properties. Each is remembered; Markdown, JSON and YAML start rendered.
const DEFAULTS = { md: true, json: true, yaml: true, svg: false, img: false }

function read(k) {
  try {
    const v = localStorage.getItem('v.render.' + k)
    return v === null ? DEFAULTS[k] : v === '1'
  } catch {
    return DEFAULTS[k]
  }
}

export const view = $state({ md: read('md'), json: read('json'), yaml: read('yaml'), svg: read('svg'), img: read('img') })

export function setRender(k, v) {
  view[k] = v
  try {
    localStorage.setItem('v.render.' + k, v ? '1' : '0')
  } catch {}
}

// What kind of file this is for the switch: md, json, yaml, svg, img, or '' when there is nothing to switch.
export function kindOf(path) {
  if (/\.(md|markdown|mdx)$/i.test(path ?? '')) return 'md'
  if (/\.svg$/i.test(path ?? '')) return 'svg'
  if (/\.json$/i.test(path ?? '')) return 'json'
  if (/\.ya?ml$/i.test(path ?? '')) return 'yaml'
  if (/\.(png|jpe?g|gif|webp|bmp|ico|avif)$/i.test(path ?? '')) return 'img'
  return ''
}

export const toggleRender = (path) => {
  const k = kindOf(path)
  if (k) setRender(k, !view[k])
  return !!k
}
