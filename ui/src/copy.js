// Put text on the clipboard. The clipboard API works on a click in most webviews; where it does
// not, a temporary text field and execCommand do.
export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    const t = document.createElement('textarea')
    t.value = text
    document.body.appendChild(t)
    t.select()
    document.execCommand('copy')
    t.remove()
  }
}
