// What a file shows as: Markdown rendered or as source, an SVG drawn or as code, a picture drawn or
// as its properties. Each is remembered; Markdown starts rendered, the others as text.
const DEFAULTS = { md: true, svg: false, img: false }

function read(k) {
  try {
    const v = localStorage.getItem('v.render.' + k)
    return v === null ? DEFAULTS[k] : v === '1'
  } catch {
    return DEFAULTS[k]
  }
}

export const view = $state({ md: read('md'), svg: read('svg'), img: read('img') })

export function setRender(k, v) {
  view[k] = v
  try {
    localStorage.setItem('v.render.' + k, v ? '1' : '0')
  } catch {}
}

// What kind of file this is for the switch: md, svg, img, or '' when there is nothing to switch.
export function kindOf(path) {
  if (/\.(md|markdown|mdx)$/i.test(path ?? '')) return 'md'
  if (/\.svg$/i.test(path ?? '')) return 'svg'
  if (/\.(png|jpe?g|gif|webp|bmp|ico|avif)$/i.test(path ?? '')) return 'img'
  return ''
}

export const toggleRender = (path) => {
  const k = kindOf(path)
  if (k) setRender(k, !view[k])
  return !!k
}
