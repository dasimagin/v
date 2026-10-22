<script>
  import { onMount, untrack } from 'svelte'
  import { saveFile, highlightText } from './api.js'

  // The file as text you can change. A transparent <textarea> lies over the highlighted lines, so
  // the colors show through while you type (they catch up a moment after you stop).
  let { file, text = $bindable(''), dirty = $bindable(false), query = '', mode = $bindable('regexp'), onescape } = $props()
  const mod = /Mac/.test(navigator.platform) ? '⌘' : 'Ctrl+'

  // svelte-ignore state_referenced_locally
  let saved = $state(file.text)
  // svelte-ignore state_referenced_locally
  let mtime = $state(file.mtime)
  // svelte-ignore state_referenced_locally
  let hl = $state(file.lines) // the colored lines of the text as it was last colored
  let err = $state('')
  let busy = $state(false)
  let note = $state('')
  let box

  // svelte-ignore state_referenced_locally
  text = file.text
  $effect(() => {
    dirty = text !== saved
  })

  const lines = $derived(text.split('\n'))

  // ----- search: the omnibox holds the query. All matches are marked, one is the current.
  const LH = 12.5 * 1.55 // a line's height in px
  const q = $derived(query.startsWith('/') ? '' : query)
  // regexp: the query is a regular expression (ignoring case unless it has a capital); if it is not one, the
  // text itself. fuzzy: a line matches if the query's letters are in it in order; each such line is one match.
  // A match is a group of runs of text; the current match is a group.
  const matches = $derived.by(() => {
    if (!q) return []
    const smart = q === q.toLowerCase()
    const out = []
    if (mode === 'fuzzy') {
      const want = smart ? q.toLowerCase() : q
      const compact = want.replace(/\s+/g, '')
      let offset = 0
      for (const line of text.split('\n')) {
        const hay = smart ? line.toLowerCase() : line
        const runs = []
        let k = 0
        for (let i = 0; i < hay.length && k < compact.length; i++) {
          if (hay[i] === compact[k]) {
            const last = runs[runs.length - 1]
            if (last && last.at + last.len === offset + i) last.len++
            else runs.push({ at: offset + i, len: 1 })
            k++
          }
        }
        if (compact && k === compact.length) out.push(runs)
        offset += line.length + 1
        if (out.length >= 2000) break
      }
      return out
    }
    const flags = smart ? 'gi' : 'g'
    let re
    try {
      re = new RegExp(q, flags)
    } catch {
      re = new RegExp(q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), flags)
    }
    for (const m of text.matchAll(re)) {
      if (!m[0]) continue // an empty match marks nothing
      out.push([{ at: m.index, len: m[0].length }])
      if (out.length >= 2000) break
    }
    return out
  })
  const marks = $derived.by(() => {
    const starts = [0]
    for (let i = 0; i < text.length; i++) if (text[i] === '\n') starts.push(i + 1)
    const out = []
    matches.forEach((runs, group) => {
      for (const { at, len } of runs) {
        let lo = 0
        let hi = starts.length - 1
        while (lo < hi) {
          const mid = (lo + hi + 1) >> 1
          if (starts[mid] <= at) lo = mid
          else hi = mid - 1
        }
        let col = 0 // the column on screen: a tab runs to the next multiple of 4
        for (const ch of text.slice(starts[lo], at)) col = ch === '\t' ? (Math.floor(col / 4) + 1) * 4 : col + 1
        out.push({ group, line: lo, col, len: text.slice(at, at + len).split('\n')[0].length })
      }
    })
    return out
  })
  let cur = $state(0)
  let scroller
  let cw = $state(7.5) // the width of one character of the editor's font, measured once it is on the page
  onMount(() => {
    const probe = document.createElement('span')
    probe.style.cssText = 'position:absolute;visibility:hidden;white-space:pre;font:12.5px var(--mono)'
    probe.textContent = 'M'.repeat(100)
    document.body.appendChild(probe)
    cw = probe.getBoundingClientRect().width / 100
    probe.remove()
  })
  function reveal(k) {
    const m = marks.find((x) => x.group === k)
    if (!m || !scroller) return
    const top = m.line * LH
    if (top < scroller.scrollTop + 8 || top > scroller.scrollTop + scroller.clientHeight - 48) scroller.scrollTop = Math.max(top - scroller.clientHeight / 3, 0)
  }
  $effect(() => {
    q // a new query starts at its first match
    untrack(() => {
      cur = 0
      reveal(0)
    })
  })
  // Next (1) or previous (-1) match.
  export function find(dir) {
    if (!matches.length) return
    cur = (cur + dir + matches.length) % matches.length
    reveal(cur)
  }
  // A line that was not touched keeps its colors; the rest is plain until the next coloring.
  const segs = (i) => {
    const colored = hl[i]
    const plain = lines[i]
    return colored && colored.map((s) => s.t).join('') === plain ? colored : [{ t: plain }]
  }

  let timer
  let seq = 0
  function changed() {
    clearTimeout(timer)
    timer = setTimeout(async () => {
      const my = ++seq
      const v = await highlightText(file.abs, text)
      if (my === seq && v.length) hl = v
    }, 250)
  }

  async function save() {
    if (busy || !dirty) return
    busy = true
    err = ''
    try {
      mtime = await saveFile(file.abs, text, file.crlf, mtime)
      saved = text
      note = 'Saved'
      setTimeout(() => (note = ''), 1500)
    } catch (e) {
      err = String(e).replace(/^Error: /, '')
    } finally {
      busy = false
    }
  }

  function key(e) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onescape?.()
    } else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
      e.preventDefault()
      save()
    } else if (e.key === 'Tab' && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
      e.preventDefault()
      insert('\t')
    } else if (e.key === 'Enter' && !e.metaKey && !e.ctrlKey && !e.shiftKey) {
      // The new line starts where the one above did.
      e.preventDefault()
      const at = box.selectionStart
      const lineStart = text.lastIndexOf('\n', at - 1) + 1
      insert('\n' + (text.slice(lineStart, at).match(/^[ \t]*/)?.[0] ?? ''))
    }
  }

  // Insert at the caret through the textarea, so that undo keeps working.
  function insert(s) {
    box.focus()
    if (!document.execCommand('insertText', false, s)) {
      box.setRangeText(s, box.selectionStart, box.selectionEnd, 'end')
      text = box.value
      changed()
    }
  }
</script>

<div class="ed">
  {#if err}<div class="err">{err}</div>{/if}
  <div class="status">
    <span class:dirty>{dirty ? '● unsaved' : note || 'saved'}{#if q}{' '}· {matches.length ? `${cur + 1}/${matches.length}` : 'no matches'}{/if}</span>
    <span class="seg" role="group" aria-label="How to search">
      <button class:on={mode === 'regexp'} title="A regular expression (Tab)" onclick={() => (mode = 'regexp')} onmousedown={(e) => e.preventDefault()}>regexp</button>
      <button class:on={mode === 'fuzzy'} title="The letters in order (Tab)" onclick={() => (mode = 'fuzzy')} onmousedown={(e) => e.preventDefault()}>fuzzy</button>
    </span>
    <button class="save" disabled={!dirty || busy} onclick={save} title="Save the file">{busy ? 'Saving…' : mod + 'S'}</button>
  </div>
  <div class="scroll" bind:this={scroller}>
    <div class="gutter" aria-hidden="true">{#each lines as _, i}<div>{i + 1}</div>{/each}</div>
    <div class="stack">
      {#each marks as m}<span class="mark" class:cur={m.group === cur} style="top:{m.line * LH}px; left:{m.col * cw}px; width:{m.len * cw}px"></span>{/each}
      <pre class="hl" aria-hidden="true">{#each lines as _, i}<div>{#each segs(i) as s}<span class={s.c ? 't-' + s.c : ''}>{s.t}</span>{/each}{#if !lines[i]}&nbsp;{/if}</div>{/each}</pre>
      <textarea
        bind:this={box}
        bind:value={text}
        oninput={changed}
        onkeydown={key}
        spellcheck="false"
        autocapitalize="off"
        autocomplete="off"
        wrap="off"
        aria-label="Edit {file.path}"
      ></textarea>
    </div>
  </div>
</div>

<style>
  .ed { flex: 1; min-height: 0; display: flex; flex-direction: column; }
  .status { flex: none; display: flex; align-items: center; justify-content: space-between; padding: 6px 4px; color: var(--dim); font-size: 12px; }
  .status .dirty { color: var(--accent2); }
  .seg { display: flex; padding: 2px; margin-left: auto; margin-right: 8px; background: var(--panel); border: 1px solid var(--line); border-radius: 7px; font: 12px 'Inter', sans-serif; }
  .seg button { background: none; border: 0; border-radius: 5px; padding: 0 10px; color: var(--dim); cursor: pointer; }
  .seg button:hover { color: var(--text); }
  .seg button.on { background: var(--accent-soft); color: var(--accent); }
  .save { background: var(--accent-soft); color: var(--accent); border: 0; border-radius: 6px; padding: 2px 10px; font: 12px var(--mono); cursor: pointer; }
  .save:disabled { opacity: 0.45; cursor: default; }
  .err { margin: 4px; padding: 6px 10px; border: 1px solid color-mix(in srgb, var(--c-deleted) 45%, var(--line)); border-radius: 6px; color: var(--c-deleted); font-size: 12.5px; }
  .scroll { flex: 1; min-height: 0; overflow: auto; display: flex; align-items: flex-start; font: 12.5px/1.55 var(--mono); }
  .gutter { position: sticky; left: 0; z-index: 1; flex: none; padding: 0 12px 0 4px; min-width: 4.5ch; text-align: right; color: var(--dim); opacity: 0.8; background: var(--bg); user-select: none; }
  .stack { position: relative; flex: 1 0 auto; min-width: 0; }
  .mark { position: absolute; height: 1.55em; border-radius: 2px; background: color-mix(in srgb, var(--accent2) 28%, transparent); pointer-events: none; }
  .mark.cur { background: color-mix(in srgb, var(--accent2) 60%, transparent); outline: 1px solid var(--accent2); }
  .hl { margin: 0; font: inherit; color: var(--strong); white-space: pre; tab-size: 4; padding-right: 24px; min-height: 100%; }
  textarea {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    margin: 0;
    padding: 0;
    border: 0;
    outline: 0;
    resize: none;
    overflow: hidden;
    background: transparent;
    color: transparent;
    caret-color: var(--strong);
    font: inherit;
    white-space: pre;
    tab-size: 4;
  }
  textarea::selection { background: color-mix(in srgb, var(--accent) 35%, transparent); color: transparent; }
</style>
