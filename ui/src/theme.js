
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

// The Dock icon is the V (the home logo: bright gradient, soft halo or shadow) on a
// rounded square in the theme's background: dark on a dark theme, light on a light
// one. It is drawn here and handed to Go.
const V = new Path2D('M8 6 H52 L96 112 L140 6 H184 L118 168 H74 Z')
function drawIcon(p) {
  if (!window.vIcon) return
  const size = 1024
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

  // The V spans 176 x 162 units; place it in the middle of the square. The icon is a still,
  // so it takes the purple stretch of the home logo's gradient.
  const k = (body * 0.52) / 176
  g.save()
  g.translate(size / 2 - 96 * k, size / 2 - 87 * k)
  g.scale(k, k)
  const fill = g.createLinearGradient(8, 6, 184, 168)
  ;[[0, '#61afef'], [0.3, '#8c97e7'], [0.55, '#b27fe6'], [0.8, '#e06cdd'], [1, '#a98be8']].forEach(([o, col]) => fill.addColorStop(o, col))
  g.shadowColor = p.dark ? 'rgba(255,255,255,0.35)' : 'rgba(31,36,48,0.25)'
  g.shadowBlur = 8 * k * 1.6
  g.shadowOffsetY = p.dark ? 0 : 2 * k
  g.fillStyle = fill
  g.fill(V)
  g.shadowColor = 'transparent'
  const shine = g.createLinearGradient(0, 6, 0, 168)
  shine.addColorStop(0, `rgba(255,255,255,${p.dark ? 0.5 : 0.2})`)
  shine.addColorStop(0.55, 'rgba(255,255,255,0)')
  g.fillStyle = shine
  g.fill(V)
  g.restore()
  window.vIcon(c.toDataURL('image/png').split(',')[1])
}

// Go tells where the window buttons are (see windowBarMetrics), so the header
// lines up with them.
window.vSetBar = (cy, right) => {
  const root = document.documentElement.style
  if (cy > 0) root.setProperty('--bar-cy', cy + 'px')
  if (right > 0) root.setProperty('--bar-left', right + 'px')
}
