import logo from './logo.svg?raw'

// A palette is {bg, panel, fg, muted, accent, accent2, dark} from the chosen theme; the
// page's other colors are mixed from it in style.css.
export function applyPalette(p) {
  const root = document.documentElement
  root.style.setProperty('--bg', p.bg)
  root.style.setProperty('--panel', p.panel)
  root.style.setProperty('--strong', p.fg)
  root.style.setProperty('--dim', p.muted)
  root.style.setProperty('--accent', p.accent)
  root.style.setProperty('--accent2', p.accent2)
  for (const [k, v] of Object.entries(p.syntax ?? {})) root.style.setProperty(`--c-${k}`, v)
  root.dataset.mode = p.dark ? 'dark' : 'light'
  root.style.colorScheme = p.dark ? 'dark' : 'light'
  // The window's own title bar has to follow too; only Go can set it.
  window.vChrome?.(p.dark, p.bg)
  drawIcon(p)
}

// The Dock icon is the V on a rounded square in the theme's background: dark on
// a dark theme, light on a light one. It is drawn here and handed to Go.
function drawIcon(p) {
  if (!window.vIcon) return
  const size = 1024
  const svg = logo.replace(/<style>.*?<\/style>/, `<style>${p.dark ? '.on-light' : '.on-dark'}{display:none}</style>`)
  const img = new Image()
  img.onload = () => {
    const c = document.createElement('canvas')
    c.width = c.height = size
    const g = c.getContext('2d')
    const body = size * 0.8 // the Dock leaves a margin around the icon
    const x = (size - body) / 2
    g.beginPath()
    g.roundRect(x, x, body, body, body * 0.225)
    g.fillStyle = p.bg
    g.fill()
    g.lineWidth = 4
    g.strokeStyle = p.dark ? 'rgba(255,255,255,0.12)' : 'rgba(0,0,0,0.12)'
    g.stroke()
    const v = body * 0.56
    g.drawImage(img, (size - v) / 2, (size - v) / 2, v, v)
    window.vIcon(c.toDataURL('image/png').split(',')[1])
  }
  img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg)
}

// Go tells where the window buttons are (see windowBarMetrics), so the header
// lines up with them.
window.vSetBar = (cy, right) => {
  const root = document.documentElement.style
  if (cy > 0) root.setProperty('--bar-cy', cy + 'px')
  if (right > 0) root.setProperty('--bar-left', right + 'px')
}
