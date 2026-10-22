<script>
  import { untrack } from 'svelte'
  import Text from './Text.svelte'
  import Compose from './Compose.svelte'
  import { fuzzy } from './filter.js'
  import { prView, prReply, prComment, prTalk, prResolve, prDelete, prSubmit, prEdit, prChecks, openURL } from './api.js'

  // A pull request: on the left its files (as a tree or a flat list) or its comments, on the
  // right the conversation or the diff of a file, where you write, answer, resolve and delete
  // comments and, from the Review button, send the review.
  let { pr = $bindable(), query = '' } = $props()

  const stored = (k, d) => {
    try {
      return localStorage.getItem(k) ?? d
    } catch {
      return d
    }
  }
  const mod = /Mac/.test(navigator.platform) ? '⌘' : 'Ctrl+'
  let tab = $state('overview') // overview | files | comments | ci
  let flat = $state(stored('v.prflat', '0') === '1')
  let closed = $state({}) // folders the reader folded, by path
  let showResolved = $state(true)
  let cur = $state('') // '' is the conversation; otherwise the open file
  let focus = $state('') // a thread to scroll to
  let view = $state(null)
  let hi = $state(0) // the row the arrows are on
  let busy = $state(false)
  let err = $state('')
  let compose = $state(null) // { kind: line | reply, ... } while a comment is being written
  let confirm = $state('') // the comment whose Delete was clicked once
  let pane

  // The list on the left can be folded into a thin rail of mode icons with the button in its corner.
  let shown = $state(stored('v.prpanel', '1') === '1')
  $effect(() => {
    try {
      localStorage.setItem('v.prpanel', shown ? '1' : '0')
    } catch {}
  })

  // ----- CI: the checks GitHub reports, loaded when the PR opens and again on demand
  let checks = $state(null) // null until the first answer
  let checksErr = $state('')
  let checksBusy = $state(false)
  let selCheck = $state('')
  async function loadChecks() {
    checksBusy = true
    checksErr = ''
    try {
      checks = await prChecks()
    } catch (e) {
      checksErr = String(e).replace(/^Error: /, '')
      checks ??= []
    } finally {
      checksBusy = false
    }
  }
  untrack(loadChecks)
  const ORDER = { fail: 0, cancel: 1, pending: 2, pass: 3, skipping: 4 }
  const checkKey = (c) => `${c.workflow}/${c.name}`
  const sums = $derived.by(() => {
    const n = { pass: 0, fail: 0, pending: 0, other: 0, all: checks?.length ?? 0 }
    for (const c of checks ?? []) n[c.bucket === 'pass' || c.bucket === 'fail' || c.bucket === 'pending' ? c.bucket : 'other']++
    return n
  })
  // The one word for all the checks: failing wins over pending, pending over passing.
  const ciState = $derived(!checks || !checks.length ? '' : sums.fail ? 'fail' : sums.pending ? 'pending' : 'pass')

  // ----- the list on the left
  function buildTree(files) {
    const root = { path: '', dirs: new Map(), files: [] }
    for (const f of files) {
      const parts = f.path.split('/')
      let n = root
      for (let i = 0; i < parts.length - 1; i++) {
        if (!n.dirs.has(parts[i])) n.dirs.set(parts[i], { name: parts[i], path: parts.slice(0, i + 1).join('/'), dirs: new Map(), files: [] })
        n = n.dirs.get(parts[i])
      }
      n.files.push(f)
    }
    return root
  }
  const total = (n) => {
    let s = { adds: 0, dels: 0, threads: 0 }
    for (const f of n.files) (s.adds += f.adds), (s.dels += f.dels), (s.threads += f.threads)
    for (const d of n.dirs.values()) {
      const t = total(d)
      ;(s.adds += t.adds), (s.dels += t.dels), (s.threads += t.threads)
    }
    return s
  }
  function walk(n, depth, out, filtering) {
    for (let d of [...n.dirs.values()].sort((a, b) => a.name.localeCompare(b.name))) {
      let name = d.name
      while (d.files.length === 0 && d.dirs.size === 1) {
        d = [...d.dirs.values()][0] // a chain of folders with nothing else in them is one row
        name += '/' + d.name
      }
      const open = filtering || !closed[d.path]
      out.push({ kind: 'dir', key: d.path, label: name, depth, open, ...total(d) })
      if (open) walk(d, depth + 1, out, filtering)
    }
    for (const f of [...n.files].sort((a, b) => a.path.localeCompare(b.path))) {
      out.push({ kind: 'file', key: f.path, label: f.path.split('/').pop(), depth, ...f })
    }
  }

  const rows = $derived.by(() => {
    const q = query.trim()
    if (tab === 'overview') return []
    if (tab === 'ci') {
      return (checks ?? [])
        .filter((c) => !q || fuzzy(q, `${c.workflow} ${c.name}`))
        .sort((a, b) => (ORDER[a.bucket] ?? 5) - (ORDER[b.bucket] ?? 5))
        .map((c) => ({ kind: 'check', key: checkKey(c), ...c }))
    }
    if (tab === 'comments') {
      const ts = pr.threads.filter((t) => (showResolved || !t.resolved) && (!q || fuzzy(q, `${t.path} ${t.preview} ${t.author}`)))
      return ts.map((t) => ({ kind: 'thread', key: t.id, ...t }))
    }
    const files = pr.files.filter((f) => !q || fuzzy(q, f.path))
    const out = []
    if (flat) for (const f of files) out.push({ kind: 'file', key: f.path, label: f.path, depth: 0, ...f })
    else walk(buildTree(files), 0, out, !!q)
    return out
  })
  const isOn = (r) => (r.kind === 'check' ? selCheck === r.key : r.kind === 'file' ? cur === r.key : r.kind === 'thread' ? focus === r.key : false)

  $effect(() => {
    if (hi >= rows.length) hi = Math.max(rows.length - 1, 0)
  })

  // Overview shows the conversation; Files and Comments show a file, so one is opened if none is.
  const TABS = ['overview', 'files', 'comments', 'ci']
  function goTab(t) {
    if (t === tab) return
    tab = t
    compose = null
    if (t === 'ci') {
      const first = untrack(() => rows.find((r) => r.kind === 'check'))
      if (first && !selCheck) selCheck = first.key
    } else if (t === 'overview') {
      cur = ''
      focus = ''
      view = null
      load()
    } else if (cur === '') {
      const first = untrack(() => rows.find((r) => r.kind === 'file' || r.kind === 'thread'))
      if (first) activate(first)
    }
  }

  // ----- the view on the right
  let seq = 0
  async function load() {
    const my = ++seq
    const v = await prView(cur)
    if (my !== seq) return
    view = v
    if (focus) queueMicrotask(() => pane?.querySelector(`[data-thread="${CSS.escape(focus)}"]`)?.scrollIntoView({ block: 'center' }))
  }
  untrack(load)

  function activate(r) {
    compose = null
    confirm = ''
    if (r.kind === 'dir') {
      closed[r.key] = r.open
      return
    }
    if (r.kind === 'check') {
      selCheck = r.key
      return
    }
    if (r.kind === 'file') (cur = r.key), (focus = '')
    else (cur = r.path), (focus = r.key)
    view = view && null
    load()
  }
  function pick(i) {
    hi = i
    activate(rows[i])
  }

  export const showTab = (t) => goTab(t)

  // The omnibox forwards its keys here.
  export function key(e, text) {
    const to = (i) => {
      hi = Math.max(0, Math.min(rows.length - 1, i))
      if (rows[hi] && rows[hi].kind !== 'dir') activate(rows[hi])
    }
    switch (e.key) {
      case 'ArrowDown': to(hi + 1); return true
      case 'ArrowUp': to(hi - 1); return true
      case 'ArrowRight':
      case 'ArrowLeft': {
        if (text) return false
        const r = rows[hi]
        if (r?.kind === 'dir' && tab === 'files') {
          closed[r.key] = e.key === 'ArrowLeft'
        } else goTab(TABS[Math.max(0, Math.min(TABS.length - 1, TABS.indexOf(tab) + (e.key === 'ArrowRight' ? 1 : -1)))])
        return true
      }
      case 'Enter':
        if (text.startsWith('/')) return false
        if (rows[hi]?.kind === 'dir') activate(rows[hi])
        return true
    }
    return false
  }

  // ----- actions: each reads the PR again and shows its new state
  async function run(fn) {
    busy = true
    err = ''
    try {
      pr = await fn()
      compose = null
      confirm = ''
      await load()
      return true
    } catch (e) {
      err = String(e).replace(/^Error: /, '')
      return false
    } finally {
      busy = false
    }
  }
  const reply = (t, text) => run(() => prReply(t.id, text))
  const commentLine = (text) => {
    const c = compose
    return run(() => prComment(c.path, c.start, c.line, text))
  }
  const resolve = (t) => run(() => prResolve(t.id, !t.resolved))
  function del(c, conversation) {
    if (confirm !== c.id) {
      confirm = c.id
      setTimeout(() => confirm === c.id && (confirm = ''), 4000)
      return
    }
    run(() => prDelete(c.id, conversation))
  }
  // The panel at the end of the conversation: one message and the verdict. Comment sends the
  // pending review with it, or, with nothing pending, posts it as a comment on the PR.
  let draft = $state('')

  // The title and the description can be changed by the one who opened the PR.
  let editing = $state(false)
  let eTitle = $state('')
  let eBody = $state('')
  const mine = $derived(!!pr.viewer && pr.viewer === pr.author)
  function startEdit() {
    eTitle = pr.title
    eBody = view?.body ?? ''
    editing = true
  }
  async function saveEdit() {
    if (await run(() => prEdit(eTitle, eBody))) editing = false
  }
  function editKey(e) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      saveEdit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      editing = false
    }
  }
  async function send(what) {
    const text = draft.trim()
    let go
    if (what === 'approve') go = () => prSubmit(1, text)
    else if (what === 'changes') go = () => prSubmit(2, text)
    else if (what === 'discard') {
      if (confirm !== 'discard') {
        confirm = 'discard'
        setTimeout(() => confirm === 'discard' && (confirm = ''), 4000)
        return
      }
      go = () => prSubmit(3, '')
    } else go = pr.pending > 0 ? () => prSubmit(0, text) : () => prTalk(text, false)
    if (await run(go) && what !== 'discard') draft = ''
  }
  function draftKey(e) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      send('comment')
    }
  }

  // A click on a gutter "+" starts a comment on that line; shift-click on another one makes a range.
  function gutter(e, no) {
    if (e.shiftKey && compose?.kind === 'line' && compose.path === cur) {
      compose = { ...compose, start: Math.min(compose.start, no), line: Math.max(compose.line, no) }
    } else compose = { kind: 'line', path: cur, start: no, line: no }
  }
  const inRange = (no) => compose?.kind === 'line' && compose.path === cur && no >= compose.start && no <= compose.line
  // The numbers on the Overview: the size of the change, how much was said, how much is still open.
  const totals = $derived({
    adds: pr.files.reduce((n, f) => n + f.adds, 0),
    dels: pr.files.reduce((n, f) => n + f.dels, 0),
    comments: pr.talk + pr.threads.reduce((n, t) => n + t.count, 0),
    open: pr.threads.filter((t) => !t.resolved).reduce((n, t) => n + t.count, 0), // comments in threads still open
  })
  // Two ways to read a diff: one column, or old and new side by side.
  let dstyle = $state(stored('v.prdiff', 'unified'))
  $effect(() => {
    try {
      localStorage.setItem('v.prdiff', dstyle)
    } catch {}
  })
  // Side by side: a run of removed lines is set against the run of added lines that follows it.
  const splitRows = $derived.by(() => {
    const ls = view?.lines ?? []
    const out = []
    for (let i = 0; i < ls.length; ) {
      const l = ls[i]
      if (l.k === 'del') {
        const dels = []
        const adds = []
        while (i < ls.length && ls[i].k === 'del') dels.push(ls[i++])
        while (i < ls.length && ls[i].k === 'add') adds.push(ls[i++])
        for (let k = 0; k < Math.max(dels.length, adds.length); k++) out.push({ l: dels[k] ?? null, r: adds[k] ?? null })
      } else if (l.k === 'add') {
        out.push({ l: null, r: l })
        i++
      } else if (l.k === 'ctx') {
        out.push({ l, r: l })
        i++
      } else {
        out.push({ full: l })
        i++
      }
    }
    return out
  })
  // The open file's numbers for the bar, and a way to step to the next file (or thread, in Comments).
  const curFile = $derived(pr.files.find((f) => f.path === cur))
  const steps = $derived(rows.filter((r) => r.kind === 'file' || r.kind === 'thread'))
  const stepAt = $derived(Math.max(steps.findIndex(isOn), 0))
  function step(d) {
    if (!steps.length) return
    const r = steps[(stepAt + d + steps.length) % steps.length]
    hi = rows.indexOf(r)
    activate(r)
  }
  const addLabel = $derived(pr.immediate ? 'Comment' : 'Add to review')
</script>

<section class="pr">
  <div class="side" class:shown>
    {#if !shown}
      <div class="rail" role="tablist" aria-label="PR sections">
        <button class="fold" title="Show the list" onclick={() => (shown = true)} onmousedown={(e) => e.preventDefault()}>
          <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M5.5 3 10.5 8l-5 5-1-1L8.5 8 4.5 4l1-1Z"/></svg>
        </button>
        {#each [['overview', 'Main'], ['files', 'Files'], ['comments', 'Threads'], ['ci', 'CI']] as [t, name] (t)}
          <button role="tab" aria-selected={tab === t} class:on={tab === t} title={name} onclick={() => goTab(t)} onmousedown={(e) => e.preventDefault()}>
            {#if t === 'overview'}
              <svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true"><path fill="currentColor" d="M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13Zm-.75 3h1.5v1.5h-1.5V4.5Zm0 3h1.5v4h-1.5v-4Z"/></svg>
            {:else if t === 'files'}
              <svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true"><path fill="currentColor" d="M3 1.5h6l4 4v9H3v-13Zm5.25 1.2V6h3.3L8.25 2.7ZM5 8h6v1.2H5V8Zm0 2.6h6v1.2H5v-1.2Z"/></svg>
            {:else if t === 'comments'}
              <svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true"><path fill="currentColor" d="M2 2.5h12v8H8.5L5 13.5v-3H2v-8Zm2.2 2.3v1.1h7.6V4.8H4.2Zm0 2.4v1.1h5V7.2h-5Z"/></svg>
            {:else}
              <svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true"><path fill="currentColor" d="M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13Zm3.1 4.6-3.9 4.2-2.3-2.2 1-1 1.3 1.3 2.9-3.1 1 .8Z"/></svg>
            {/if}
          </button>
        {/each}
      </div>
    {:else}
      <button class="pin" title="Hide the list" onclick={() => (shown = false)} onmousedown={(e) => e.preventDefault()}>
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M10.5 3 5.5 8l5 5 1-1L7.5 8l4-4-1-1Z"/></svg>
      </button>
      <div class="modes" role="tablist">
        <button role="tab" aria-selected={tab === 'overview'} class:on={tab === 'overview'} onclick={() => goTab('overview')} onmousedown={(e) => e.preventDefault()}>Main</button>
        <button role="tab" aria-selected={tab === 'files'} class:on={tab === 'files'} onclick={() => goTab('files')} onmousedown={(e) => e.preventDefault()}>Files</button>
        <button role="tab" aria-selected={tab === 'comments'} class:on={tab === 'comments'} onclick={() => goTab('comments')} onmousedown={(e) => e.preventDefault()}>Threads</button>
        <button role="tab" aria-selected={tab === 'ci'} class:on={tab === 'ci'} onclick={() => goTab('ci')} onmousedown={(e) => e.preventDefault()}>CI{#if ciState}<i class="dot {ciState}"></i>{/if}</button>
      </div>
      {#if tab !== 'overview'}
        <div class="sub">
          {#if tab === 'ci'}
            <button class="mini" disabled={checksBusy} onclick={loadChecks} onmousedown={(e) => e.preventDefault()}>{checksBusy ? 'Checking…' : 'Refresh'}</button>
          {:else if tab === 'files'}
            <span class="seg" role="group" aria-label="Files layout">
              <button class:on={!flat} title="Tree" onclick={() => (flat = false)} onmousedown={(e) => e.preventDefault()}>
                <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M1.5 2h5v2.2h-5V2Zm1.4 2.2h1.2V12H2.9V4.2Zm1.2 2.2h1.6v1.2H4.1V6.4Zm0 4.4h1.6V12H4.1v-1.2ZM7 5.4h7.5v2.2H7V5.4Zm0 4.4h7.5V12H7V9.8Z"/></svg>
              </button>
              <button class:on={flat} title="Flat list" onclick={() => (flat = true)} onmousedown={(e) => e.preventDefault()}>
                <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path fill="currentColor" d="M2 3h12v2H2V3Zm0 4h12v2H2V7Zm0 4h12v2H2v-2Z"/></svg>
              </button>
            </span>
          {:else}
            <span class="seg" role="group" aria-label="Which threads">
              <button class="txt" class:on={!showResolved} title="Only threads that are not resolved" onclick={() => (showResolved = false)} onmousedown={(e) => e.preventDefault()}>open</button>
              <button class="txt" class:on={showResolved} title="All threads, resolved too" onclick={() => (showResolved = true)} onmousedown={(e) => e.preventDefault()}>all</button>
            </span>
          {/if}
        </div>
      {/if}

    <div class="list">
      {#if tab === 'overview'}
        <div class="facts">
          <div class="fact"><span>Author</span><b>@{pr.author}</b></div>
          <div class="fact"><span>Branch</span><b>{pr.head} → {pr.base}</b></div>
          <div class="fact"><span>State</span><b>{pr.state}{pr.draft ? ', draft' : ''}</b></div>
          {#if pr.labels.length}<div class="fact"><span>Labels</span><b>{pr.labels.join(', ')}</b></div>{/if}
          <button class="fact link" onclick={() => goTab('files')} onmousedown={(e) => e.preventDefault()}>
            <span>Changes</span><b><span class="add">+{totals.adds}</span> <span class="del">−{totals.dels}</span></b>
          </button>
          <button class="fact link" title="not resolved / all comments" onclick={() => goTab('comments')} onmousedown={(e) => e.preventDefault()}>
            <span>Comments</span><b><span class:warn={totals.open > 0}>{totals.open}</span>/{totals.comments}</b>
          </button>
          {#if checks}
            <button class="fact link" onclick={() => goTab('ci')} onmousedown={(e) => e.preventDefault()}>
              <span>Checks</span>
              <b>{#if !sums.all}none{:else if sums.fail}<span class="del">{sums.fail} failing</span>{:else if sums.pending}<span class="pend">{sums.pending} running</span>{:else}<span class="add">all {sums.pass} passed</span>{/if}</b>
            </button>
          {/if}
          {#if pr.pending}<div class="fact"><span>Pending in your review</span><b>{pr.pending}</b></div>{/if}
        </div>
      {/if}
      {#each rows as r, i (r.kind + r.key)}
        {#if r.kind === 'dir'}
          <button class="row dir" class:hi={i === hi} style="padding-left:{8 + r.depth * 14}px" onclick={() => pick(i)} onmousedown={(e) => e.preventDefault()}>
            <span class="car" class:open={r.open}>›</span>
            <span class="label">{r.label}/</span>{#if r.threads}<span class="cn">[{r.threads}]</span>{/if}
            <span class="stat"><span class="add">+{r.adds}</span> <span class="del">−{r.dels}</span></span>
          </button>
        {:else if r.kind === 'file'}
          <button class="row" class:on={isOn(r)} class:hi={i === hi} style="padding-left:{flat ? 10 : 8 + r.depth * 14 + 14}px" onclick={() => pick(i)} onmousedown={(e) => e.preventDefault()}>
            <span class="label" class:rtl={flat}>{r.label}</span>{#if r.threads}<span class="cn">[{r.threads}]</span>{/if}
            <span class="stat"><span class="add">+{r.adds}</span> <span class="del">−{r.dels}</span></span>
          </button>
        {:else if r.kind === 'check'}
          <button class="row chk" class:on={isOn(r)} class:hi={i === hi} onclick={() => pick(i)} onmousedown={(e) => e.preventDefault()}>
            <i class="st {r.bucket}"></i>
            <span class="cname">{r.name}</span>
            <span class="stat">{r.workflow}</span>
          </button>
        {:else if r.kind === 'thread'}
          <button class="row th2" class:on={isOn(r)} class:hi={i === hi} class:done={r.resolved} onclick={() => pick(i)} onmousedown={(e) => e.preventDefault()}>
            <span class="tline"><span class="tpath">{r.path.split('/').pop()}{r.line ? ':' + r.line : ''}</span>
              {#if r.pending}<span class="chip">pending</span>{/if}{#if r.resolved}<span class="chip ok">resolved</span>{/if}{#if r.outdated}<span class="chip">outdated</span>{/if}
              <span class="stat">{r.count > 1 ? r.count + ' replies' : ''}</span></span>
            <span class="prev"><b>{r.author}</b> {r.preview}</span>
          </button>
        {/if}
      {/each}
      {#if tab !== 'overview' && rows.length === 0}
        <div class="msg">Nothing matches</div>
      {/if}
    </div>

  {/if}
  </div>

  <div class="right">
    <div class="bar">
      <div class="crumb">
        {#if tab === 'ci'}
          <span class="cnum">CI</span>
          {#if sums.all}<span class="cstate"><span class="add">{sums.pass} passed</span>{#if sums.fail} · <span class="del">{sums.fail} failed</span>{/if}{#if sums.pending} · <span class="pend">{sums.pending} running</span>{/if}{#if sums.other} · {sums.other} skipped{/if}</span>{/if}
        {:else if tab !== 'overview' && cur}
          <span class="cpath" title={cur}>{cur}</span>
          {#if curFile}
            <span class="cstat"><span class="add">+{curFile.adds}</span> <span class="del">−{curFile.dels}</span></span>
            {#if curFile.threads}<span class="cstate">{curFile.threads} open thread{curFile.threads > 1 ? 's' : ''}</span>{/if}
          {/if}
        {:else}
          <span class="cnum">#{pr.number}</span><span class="cstate">{pr.state}{pr.draft ? ', draft' : ''}</span>
        {/if}
      </div>
      <div class="tools">
        {#if tab !== 'overview' && tab !== 'ci' && cur && steps.length > 1}
          <span class="seg step" role="group" aria-label="Previous and next">
            <button title="Previous" onclick={() => step(-1)} onmousedown={(e) => e.preventDefault()}>
              <svg viewBox="0 0 16 16" width="13" height="13" aria-hidden="true"><path fill="currentColor" d="M10.5 3 5.5 8l5 5 1-1L7.5 8l4-4-1-1Z"/></svg>
            </button>
            <span class="pos">{stepAt + 1}/{steps.length}</span>
            <button title="Next" onclick={() => step(1)} onmousedown={(e) => e.preventDefault()}>
              <svg viewBox="0 0 16 16" width="13" height="13" aria-hidden="true"><path fill="currentColor" d="M5.5 3 10.5 8l-5 5-1-1L8.5 8 4.5 4l1-1Z"/></svg>
            </button>
          </span>
        {/if}
        {#if tab !== 'overview' && tab !== 'ci' && cur}
          <span class="seg" role="group" aria-label="Diff style">
            <button class="txt" class:on={dstyle === 'unified'} title="One column" onclick={() => (dstyle = 'unified')} onmousedown={(e) => e.preventDefault()}>unified</button>
            <button class="txt" class:on={dstyle === 'split'} title="Old and new side by side" onclick={() => (dstyle = 'split')} onmousedown={(e) => e.preventDefault()}>split</button>
          </span>
        {/if}
      </div>
    </div>
  <div class="pane" bind:this={pane}>
    {#if err}<div class="err">{err}</div>{/if}
    {#if tab === 'ci'}
      <div class="ci">
        {#if checksErr}<div class="err">{checksErr}</div>{/if}
        {#if checks === null}<div class="msg">Asking GitHub…</div>
        {:else if !checks.length}<div class="msg">No checks on this PR.</div>
        {:else}
          {#each checks.slice().sort((a, b) => (ORDER[a.bucket] ?? 5) - (ORDER[b.bucket] ?? 5)) as c (checkKey(c))}
            <div class="check" class:sel={selCheck === checkKey(c)}>
              <i class="st {c.bucket}"></i>
              <div class="cinfo">
                <div class="ctop"><b>{c.name}</b>{#if c.workflow}<span class="wf">{c.workflow}</span>{/if}<span class="bucket {c.bucket}">{c.bucket === 'skipping' ? 'skipped' : c.bucket === 'cancel' ? 'cancelled' : c.bucket === 'pending' ? 'running' : c.bucket === 'fail' ? 'failed' : 'passed'}</span></div>
                {#if c.description}<div class="cdesc">{c.description}</div>{/if}
              </div>
              {#if c.link}<button class="mini" onclick={() => openURL(c.link)}>Open</button>{/if}
            </div>
          {/each}
        {/if}
      </div>
    {:else if !view}
      <div class="msg">Loading…</div>
    {:else if tab === 'overview'}
      <div class="convo">
        {#if editing}
          <div class="edit">
            <input class="etitle" bind:value={eTitle} onkeydown={editKey} placeholder="Title" disabled={busy} />
            <textarea class="ebody" bind:value={eBody} onkeydown={editKey} rows="10" placeholder="Description (Markdown)" disabled={busy}></textarea>
            <div class="ebar">
              <span class="vhint">{mod}Enter saves · Esc cancels</span><span class="sp"></span>
              <button class="vb" disabled={busy} onclick={() => (editing = false)}>Cancel</button>
              <button class="vb go" disabled={busy || !eTitle.trim()} onclick={saveEdit}>{busy ? 'Saving…' : 'Save'}</button>
            </div>
          </div>
        {:else}
          <div class="ptitle">{pr.title}
            {#if mine}<button class="mini" onclick={startEdit}>Edit</button>{/if}</div>
          {#if pr.labels.length}<div class="labels">{#each pr.labels as l}<span class="label-chip">{l}</span>{/each}</div>{/if}
          <div class="body"><Text text={view.body || '(no description)'} /></div>
        {/if}
        {#each view.reviews as r}
          <div class="box" class:approved={r.state === 'approved'} class:changes={r.state === 'changes requested'}>
            <div class="by"><b>{r.author}</b> <span class="badge">{r.state}</span> <span class="when">{r.when}</span></div>
            {#if r.body}<div class="text"><Text text={r.body} /></div>{/if}
          </div>
        {/each}
        {#each view.comments as c}
          <div class="box">
            <div class="by"><b>{c.author}</b> <span class="when">{c.when}</span><span class="sp"></span>
              {#if c.mine}<button class="mini danger" onclick={() => del(c, true)}>{confirm === c.id ? 'Sure?' : 'Delete'}</button>{/if}</div>
            <div class="text"><Text text={c.body} /></div>
          </div>
        {/each}
        <div class="verdict">
          <textarea bind:value={draft} onkeydown={draftKey} rows="3" disabled={busy}
            placeholder={pr.pending ? `Your review: ${pr.pending} pending comment${pr.pending > 1 ? 's' : ''} will be sent with it…` : 'Leave a comment, or review this PR…'}></textarea>
          <div class="vbar">
            {#if pr.pending}
              <button class="vlink" disabled={busy} onclick={() => send('discard')}>{confirm === 'discard' ? 'Sure? Discards ' + pr.pending : 'Discard ' + pr.pending + ' pending'}</button>
            {:else}<span class="vhint">{mod}Enter comments</span>{/if}
            <span class="sp"></span>
            <button class="vb" disabled={busy || (!draft.trim() && !pr.pending)} onclick={() => send('comment')}>Comment</button>
            <button class="vb ok" disabled={busy} onclick={() => send('approve')}>Approve</button>
            <button class="vb bad" disabled={busy || !draft.trim()} title={draft.trim() ? '' : 'Requesting changes needs a message'} onclick={() => send('changes')}>Request changes</button>
          </div>
        </div>
      </div>
    {:else if cur === ''}
      <div class="msg">Pick a file or a thread on the left</div>
    {:else}
      <div class="diff">
        {#if dstyle === 'unified'}
          {#each view.lines as l}
            <div class="dl {l.k}" class:insel={l.no && inRange(l.no)} class:top={l.no && compose?.start === l.no} class:bot={l.no && compose?.line === l.no}>
              {#if l.no && l.k !== 'del'}
                <button class="gut" title="Comment on this line (shift-click for a range)" onclick={(e) => gutter(e, l.no)}>+</button>
              {:else}<span class="gut off"></span>{/if}
              <span class="no">{l.no || ''}</span><span class="t">{l.t || ' '}</span>
            </div>
            {@render after(l)}
          {/each}
        {:else}
          {#each splitRows as row}
            {#if row.full}
              <div class="dl {row.full.k} full"><span class="t">{row.full.t || ' '}</span></div>
            {:else}
              <div class="sp2" class:insel={row.r?.no && inRange(row.r.no)} class:top={row.r?.no && compose?.start === row.r.no} class:bot={row.r?.no && compose?.line === row.r.no}>
                <div class="cell {row.l ? (row.l.k === 'del' ? 'del' : 'ctx') : 'none'}">
                  <span class="no">{row.l?.old || ''}</span><span class="t">{row.l ? row.l.t.slice(1) || ' ' : ' '}</span>
                </div>
                <div class="cell {row.r ? (row.r.k === 'add' ? 'add' : 'ctx') : 'none'}">
                  {#if row.r?.no}
                    <button class="gut" title="Comment on this line (shift-click for a range)" onclick={(e) => gutter(e, row.r.no)}>+</button>
                  {:else}<span class="gut off"></span>{/if}
                  <span class="no">{row.r?.no || ''}</span><span class="t">{row.r ? row.r.t.slice(1) || ' ' : ' '}</span>
                </div>
              </div>
              {#if row.r}{@render after(row.r)}{/if}
            {/if}
          {/each}
        {/if}
        {#if view.outdated.length}
          <div class="out">Outdated</div>
          {#each view.outdated as t}{@render thread(t)}{/each}
        {/if}
      </div>
    {/if}
  </div>
  </div>
</section>

{#snippet after(l)}
  {#each l.threads ?? [] as t}{@render thread(t)}{/each}
  {#if compose?.kind === 'line' && compose.path === cur && compose.line === l.no && l.k !== 'del'}
    <div class="inline">
      <div class="range">Comment on {compose.start < compose.line ? `lines ${compose.start}–${compose.line}` : `line ${compose.line}`}</div>
      <Compose placeholder="Leave a comment…" label={addLabel} {busy} onsubmit={commentLine} oncancel={() => (compose = null)} />
    </div>
  {/if}
{/snippet}

{#snippet thread(t)}
  <div class="thread" class:resolved={t.resolved} data-thread={t.id}>
    {#if t.resolved || t.outdated}
      <div class="flags">{t.resolved ? 'resolved' : ''}{t.resolved && t.outdated ? ' · ' : ''}{t.outdated ? 'outdated' : ''}</div>
    {/if}
    {#each t.comments as c, ci}
      <div class="tc" class:reply={ci > 0}>
        <div class="by">
          <span class="av">{(c.author || '?')[0].toUpperCase()}</span><b>{c.author}</b><span class="when">{c.when}</span>
          {#if c.pending}<span class="chip">pending</span>{/if}
          <span class="sp"></span>
          {#if c.mine}<button class="mini danger" onclick={() => del(c, false)}>{confirm === c.id ? 'Sure?' : 'Delete'}</button>{/if}
        </div>
        <div class="text"><Text text={c.body} /></div>
      </div>
    {/each}
    {#if compose?.kind === 'reply' && compose.thread === t.id}
      <div class="inline reply"><Compose placeholder="Reply…" label={pr.immediate ? 'Reply' : 'Add to review'} {busy} onsubmit={(x) => reply(t, x)} oncancel={() => (compose = null)} /></div>
    {:else}
      <div class="tact">
        <button class="mini" onclick={() => (compose = { kind: 'reply', thread: t.id })}>Reply</button>
        <button class="mini" disabled={busy} onclick={() => resolve(t)}>{t.resolved ? 'Reopen' : 'Resolve'}</button>
      </div>
    {/if}
  </div>
{/snippet}

<style>
  .pr { flex: 1; min-height: 0; width: 100%; max-width: var(--col); margin: 0 auto; display: flex; gap: 16px; padding: 4px 0 14px; }
  .right { flex: 1; min-width: 0; min-height: 0; display: flex; flex-direction: column; border-left: 1px solid var(--divider); padding-left: 16px; }
  .pane { flex: 1; min-width: 0; min-height: 0; overflow: auto; scrollbar-width: none; }
  .pane::-webkit-scrollbar { display: none; }
  .side, .pane { min-height: 0; }
  .list { flex: 1; min-height: 0; overflow: auto; scrollbar-width: none; }
  .list::-webkit-scrollbar { display: none; }
  .ptitle { color: var(--strong); font-size: 18px; font-weight: 600; line-height: 1.35; }
  .ptitle .mini { margin-left: 8px; vertical-align: middle; }
  .edit { display: flex; flex-direction: column; gap: 6px; }
  .etitle, .ebody { width: 100%; background: var(--panel); border: 1px solid var(--line); border-radius: 8px; color: var(--strong); padding: 6px 10px; outline: 0; font: 13px/1.5 'Inter', sans-serif; user-select: text; }
  .etitle { font-size: 16px; font-weight: 600; }
  .ebody { resize: vertical; font-family: var(--mono); font-size: 12.5px; }
  .etitle:focus, .ebody:focus { border-color: var(--accent); }
  .ebar { display: flex; align-items: center; gap: 6px; }
  .vb.go { background: var(--accent); color: var(--bg); }
  .side { flex: none; width: 44px; display: flex; flex-direction: column; }
  .side.shown { width: 340px; padding-right: 6px; }
  .rail { display: flex; flex-direction: column; gap: 2px; padding: 2px; background: var(--panel); border: 1px solid var(--line); border-radius: 7px; align-self: flex-start; margin: 4px 0 0 4px; }
  .rail button { display: grid; place-items: center; width: 30px; height: 30px; background: none; border: 0; border-radius: 5px; color: var(--dim); cursor: pointer; }
  .rail button:hover { color: var(--text); }
  .rail button.on { background: var(--accent-soft); color: var(--accent); }
  .pin { align-self: flex-end; display: grid; place-items: center; width: 24px; height: 22px; margin: 2px 4px 0 0; background: none; border: 0; border-radius: 5px; color: var(--dim); cursor: pointer; opacity: 0.7; }
  .pin:hover { opacity: 1; background: var(--raised); }

  /* The bar over the diff: where you are, how big it is, and how to read it. It separates the diff from the rest. */
  .bar { flex: none; display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; min-height: 28px; padding: 0 0 8px; margin-bottom: 8px; border-bottom: 1px solid var(--divider); }
  .rail .fold { height: 24px; margin-bottom: 2px; border-bottom: 1px solid var(--line); border-radius: 5px 5px 0 0; }
  .crumb { flex: 1 1 220px; min-width: 0; display: flex; align-items: baseline; gap: 8px; overflow: hidden; }
  .cpath { color: var(--strong); font: 12.5px var(--mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; direction: rtl; text-align: left; }
  .cnum { color: var(--strong); font-weight: 600; }
  .cstate { color: var(--dim); font-size: 12px; white-space: nowrap; }
  .cstat { font: 12px var(--mono); white-space: nowrap; }
  .seg.step { align-items: center; }
  .pos { padding: 0 4px; color: var(--dim); font-size: 11.5px; min-width: 3ch; text-align: center; }
  .dot { display: inline-block; width: 6px; height: 6px; margin-left: 5px; border-radius: 50%; vertical-align: middle; background: var(--dim); }
  .dot.pass, .st.pass { background: var(--c-added); }
  .dot.fail, .st.fail { background: var(--c-deleted); }
  .dot.pending, .st.pending { background: var(--accent2); animation: ping 1.2s ease-in-out infinite; }
  .st.skipping, .st.cancel { background: var(--dim); }
  @keyframes ping { 50% { opacity: 0.35; } }
  .st { flex: none; display: inline-block; width: 8px; height: 8px; border-radius: 50%; }
  .pend { color: var(--accent2); }
  .row.chk { gap: 8px; justify-content: flex-start; }
  .row.chk .cname { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--strong); }
  .ci { display: flex; flex-direction: column; gap: 6px; }
  .check { display: flex; align-items: center; gap: 10px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); }
  .check.sel { border-color: var(--accent); }
  .cinfo { flex: 1; min-width: 0; }
  .ctop { display: flex; align-items: baseline; gap: 8px; }
  .wf { color: var(--dim); font-size: 12px; }
  .bucket { margin-left: auto; font-size: 11.5px; color: var(--dim); }
  .bucket.pass { color: var(--c-added); }
  .bucket.fail { color: var(--c-deleted); }
  .bucket.pending { color: var(--accent2); }
  .cdesc { color: var(--dim); font-size: 12px; margin-top: 2px; }
  .tools { display: flex; align-items: center; gap: 8px; margin-left: auto; }
  .sub { display: flex; justify-content: flex-end; padding: 0 6px 6px; }

  /* The mode switch and the layout switch are segmented like the ones in the settings. */
  .modes, .seg { display: flex; padding: 2px; background: var(--panel); border: 1px solid var(--line); border-radius: 7px; }
  .modes { margin: 4px 6px 8px; }
  .modes > button { flex: 1; background: none; border: 0; border-radius: 5px; padding: 3px 8px; color: var(--dim); cursor: pointer; white-space: nowrap; }
  .modes > button:hover { color: var(--text); }
  .modes > button.on { background: var(--accent-soft); color: var(--accent); }
  .seg button { display: grid; place-items: center; width: 30px; height: 22px; background: none; border: 0; border-radius: 5px; color: var(--dim); cursor: pointer; }
  .seg button.txt { width: auto; padding: 0 12px; font-size: 12px; }
  .seg button:hover { color: var(--text); }
  .seg button.on { background: var(--accent-soft); color: var(--accent); }

  .row { width: 100%; display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 4px 8px; background: none; border: 0; border-radius: 6px; text-align: left; cursor: pointer; }
  .row:hover { background: var(--raised); }
  .row.hi { background: var(--raised); }
  .row.on { background: var(--accent-soft); }
  .label { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--strong); }
  .label.rtl { direction: rtl; text-align: left; }
  .label.conv { color: var(--accent); }
  .cn { flex: none; margin-left: -4px; color: var(--strong); font-size: 12.5px; }
  .dir .cn { color: var(--text); }
  .dir .label { color: var(--text); }
  .car { flex: none; width: 10px; color: var(--dim); transition: transform 0.12s; }
  .car.open { transform: rotate(90deg); }
  .stat { flex: none; margin-left: auto; color: var(--dim); font-size: 12px; }
  .add { color: var(--c-added); }
  .del { color: var(--c-deleted); }
  .th { color: var(--accent); }
  .msg { padding: 8px 10px; color: var(--dim); }
  .facts { padding: 4px 6px; display: flex; flex-direction: column; gap: 1px; }
  .fact { display: flex; justify-content: space-between; gap: 12px; padding: 4px 8px; border-radius: 6px; background: none; border: 0; text-align: left; color: var(--dim); font-size: 12.5px; }
  .fact b { color: var(--strong); font-weight: 500; text-align: right; overflow-wrap: anywhere; }
  .fact b .warn { color: var(--accent2); }
  .fact.link { cursor: pointer; }
  .fact.link:hover { background: var(--raised); }

  .row.th2 { flex-direction: column; align-items: stretch; gap: 1px; }
  .row.th2.done { opacity: 0.6; }
  .tline { display: flex; align-items: center; gap: 6px; }
  .tpath { color: var(--strong); font: 12px var(--mono); }
  .tline .stat { margin-left: auto; }
  .prev { color: var(--dim); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .prev b { color: var(--text); font-weight: 500; }
  .chip { font-size: 10.5px; padding: 0 6px; border-radius: 999px; border: 1px solid var(--line); color: var(--dim); }
  .chip.ok { color: var(--c-added); border-color: color-mix(in srgb, var(--c-added) 40%, var(--line)); }


  .err { margin-bottom: 8px; padding: 6px 10px; border: 1px solid color-mix(in srgb, var(--c-deleted) 45%, var(--line)); border-radius: 6px; color: var(--c-deleted); font-size: 12.5px; }

  .convo { display: flex; flex-direction: column; gap: 12px; user-select: text; padding-bottom: 8px; }
  .labels { display: flex; gap: 6px; }
  .label-chip { background: var(--accent-soft); color: var(--accent); border-radius: 10px; padding: 1px 8px; font-size: 12px; }
  .body { color: var(--text); line-height: 1.6; }
  .box { border: 1px solid var(--line); border-radius: var(--radius); padding: 8px 12px; }
  .box.approved { border-color: var(--c-added); }
  .box.changes { border-color: var(--c-deleted); }
  .by { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--text); }
  .sp { flex: 1; }
  .when, .badge, .soft { color: var(--dim); }
  .text { margin-top: 4px; color: var(--text); line-height: 1.55; }
  .verdict { margin-top: 6px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); padding: 8px 10px 8px; }
  .verdict:focus-within { border-color: var(--accent); }
  .verdict textarea { width: 100%; resize: vertical; min-height: 3.6em; border: 0; outline: 0; background: none; color: var(--strong); font: 13px/1.5 'Inter', sans-serif; user-select: text; }
  .verdict textarea::placeholder { color: var(--dim); }
  .vbar { display: flex; align-items: center; gap: 6px; margin-top: 4px; }
  .vhint { color: var(--dim); font-size: 11px; }
  .vb { border: 0; border-radius: 6px; padding: 4px 12px; background: var(--raised); color: var(--text); font-size: 12px; cursor: pointer; }
  .vb:hover:not(:disabled) { background: var(--accent-soft); color: var(--accent); }
  .vb.ok { background: color-mix(in srgb, var(--c-added) 18%, transparent); color: var(--c-added); }
  .vb.ok:hover:not(:disabled) { background: var(--c-added); color: var(--bg); }
  .vb.bad { background: color-mix(in srgb, var(--c-deleted) 16%, transparent); color: var(--c-deleted); }
  .vb.bad:hover:not(:disabled) { background: var(--c-deleted); color: var(--bg); }
  .vb:disabled { opacity: 0.4; cursor: default; }
  .vlink { background: none; border: 0; padding: 0; color: var(--dim); font-size: 12px; cursor: pointer; }
  .vlink:hover { color: var(--c-deleted); }

  .diff { font: 12.5px/1.55 var(--mono); user-select: text; }
  .dl { display: flex; white-space: pre; }
  .gut { flex: none; width: 18px; height: 18px; margin: 1px 4px 0 0; padding: 0; border: 0; border-radius: 4px; background: none; color: transparent; font: 600 13px/1 var(--mono); cursor: pointer; user-select: none; }
  .dl:hover .gut:not(.off) { background: var(--accent); color: var(--bg); }
  .gut.off { cursor: default; }
  .no { flex: none; width: 4.5ch; margin-right: 12px; text-align: right; color: var(--dim); opacity: 0.7; user-select: none; }
  .dl.add { background: color-mix(in srgb, var(--c-added) 14%, transparent); }
  .dl.del { background: color-mix(in srgb, var(--c-deleted) 14%, transparent); }
  /* The lines picked for a comment are one band with a bar on its left, not a box per line. */
  .insel { background: var(--accent-soft) !important; box-shadow: inset 3px 0 0 var(--accent); }
  .insel.top { border-top-right-radius: 4px; }
  .insel.bot { border-bottom-right-radius: 4px; }
  .dl.full { padding-left: calc(22px + 4.5ch + 12px); }
  .sp2 { display: grid; grid-template-columns: 1fr 1fr; white-space: pre-wrap; overflow-wrap: anywhere; }
  .cell { display: flex; min-width: 0; padding-right: 8px; }
  .cell + .cell { border-left: 1px solid var(--line); }
  .cell .t { flex: 1; min-width: 0; }
  .cell.del { background: color-mix(in srgb, var(--c-deleted) 14%, transparent); }
  .cell.del .t { color: var(--c-deleted); }
  .cell.add { background: color-mix(in srgb, var(--c-added) 14%, transparent); }
  .cell.add .t { color: var(--c-added); }
  .cell.ctx .t { color: var(--strong); }
  .cell.none { background: repeating-linear-gradient(135deg, transparent 0 6px, color-mix(in srgb, var(--line) 50%, transparent) 6px 7px); }
  .cell .gut { margin-right: 2px; }
  .sp2:hover .gut:not(.off) { background: var(--accent); color: var(--bg); }
  .cell:first-child .no { margin-left: 22px; }
  .dl.add .t { color: var(--c-added); }
  .dl.del .t { color: var(--c-deleted); }
  .dl.hunk .t { color: var(--c-func); }
  .dl.head .t { color: var(--strong); font-weight: 600; }
  .dl.meta .t { color: var(--dim); }
  .dl.ctx .t { color: var(--strong); }

  .inline { margin: 4px 0 8px calc(22px + 4.5ch + 12px); font: 13px/1.5 'Inter', sans-serif; white-space: normal; }
  .range { margin-bottom: 3px; color: var(--dim); font-size: 11.5px; }
  .thread { margin: 4px 0 8px calc(22px + 4.5ch + 12px); padding: 6px 12px; font: 13px/1.5 'Inter', sans-serif; background: var(--panel); border: 1px solid var(--line); border-left: 2px solid var(--accent); border-radius: 0 var(--radius) var(--radius) 0; white-space: normal; }
  .thread.resolved { opacity: 0.65; border-left-color: var(--dim); }
  .flags { font-size: 11px; color: var(--dim); text-transform: uppercase; letter-spacing: 0.06em; }
  .tc + .tc { margin-top: 8px; }
  /* Every answer sits at the same depth, a step to the right of the comment it answers. */
  .tc.reply { margin-left: 16px; padding-left: 10px; border-left: 2px solid var(--line); }
  .thread .inline.reply { margin: 8px 0 0 16px; }
  .av { display: grid; place-items: center; width: 18px; height: 18px; border-radius: 50%; background: linear-gradient(135deg, var(--accent), var(--accent2)); color: var(--bg); font-size: 10px; font-weight: 700; }
  .tact { display: flex; gap: 6px; margin-top: 6px; }
  .mini { background: none; border: 1px solid var(--line); border-radius: 6px; padding: 1px 9px; color: var(--dim); font-size: 11.5px; cursor: pointer; }
  .mini:hover:not(:disabled) { color: var(--text); background: var(--raised); }
  .mini.danger:hover { color: var(--c-deleted); border-color: var(--c-deleted); }
  .out { margin: 12px 0 4px; color: var(--dim); font-size: 12px; }
</style>
