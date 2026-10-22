<script>
  import { onMount } from 'svelte'
  import { start, tagline, getOptions, setOption, palette, git, branches as loadBranches, switchBranch, list, preview, file as readFile, setWorkdir, loadPR, prView as loadPRView, onAgent, send, answer, interrupt, reset, resume, quit } from './api.js'
  import { applyPalette } from './theme.js'
  import Omnibox from './Omnibox.svelte'
  import Config from './Config.svelte'
  import Picker from './Picker.svelte'
  import FileView from './FileView.svelte'
  import Chat from './Chat.svelte'
  import PRView from './PRView.svelte'
  import { visible, fuzzy } from './filter.js'
  import logo from './logo.svg?raw'

  let info = $state(null)
  let options = $state([])
  let screen = $state('home')
  let note = $state('')
  let omni
  let branchList = $state([])
  let query = $state('')
  let sel = $state(0)
  let pick = $state(null) // the list of /open and /show
  let previewLines = $state([])
  let openedFile = $state(null)
  let pr = $state(null) // the open pull request
  let prPane = $state(null)
  let messages = $state([]) // the conversation with the agent
  let busy = $state(false)
  let queue = [] // prompts typed while the agent works
  let cameFrom = 'home' // the screen to return to from a file

  const titles = { config: 'Settings', open: 'Open', show: 'History', workdir: 'Workdir', switch: 'Branches', resume: 'Sessions' }
  const prRows = $derived(
    pr
      ? [{ path: '', label: 'Conversation' }, ...pr.files.filter((f) => fuzzy(query, f.path)).map((f) => ({ ...f, label: f.path }))].filter((r) => r.path !== '' || !query.trim())
      : [],
  )
  const title = $derived(screen === 'file' ? (openedFile?.path.split('/').pop() ?? '') : screen === 'pr' ? (pr ? `PR #${pr.number}` : 'PR') : (titles[screen] ?? ''))
  const placeholders = {
    pr: 'Type to filter files · ↑↓ pick · Esc back',
    chat: 'Message the agent, or type / for commands · Esc stops a running turn',
    config: 'Search settings · ↑↓ pick · ←→ change · / for commands',
    workdir: 'Type to find a folder · ↑↓ pick · → enter · ← up · Enter choose · Esc back',
    switch: 'Type to find a branch · ↑↓ pick · Enter switch · Esc back',
    resume: 'Type to find a session · ↑↓ pick · Esc back',
    open: 'Type to find a file · ↑↓ pick · → enter or open · ← up · Esc back',
    show: 'Type to filter commits · ↑↓ pick · Esc back',
    file: '/ for commands · Esc closes the file',
  }
  const placeholder = $derived(placeholders[screen] ?? 'Ask the agent, or type / for commands')
  const picking = $derived(['open', 'show', 'workdir', 'switch', 'resume'].includes(screen))

  onMount(async () => {
    onAgent(onagent)
    info = await start()
    applyPalette(info.palette)
    options = await getOptions()
    branchList = await loadBranches()
    omni?.focus()
    // In a plain browser (npm run dev) #/open, #/config and so on jump to a screen.
    if (!window.vStart && location.hash.startsWith('#/')) {
      const [name, ...rest] = decodeURIComponent(location.hash.slice(2)).split(' ')
      if (name === 'prfile') {
        await startPR('')
        sel = 1
        loadPane()
      } else if (name === 'send') onsend(rest.join(' '))
      else onrun(name, rest.join(' '))
    }
    // The branch and the changed files move while the agent works.
    const timer = setInterval(async () => {
      info.git = await git()
      branchList = await loadBranches()
    }, 4000)
    return () => clearInterval(timer)
  })

  let noteTimer
  function flash(msg) {
    note = msg
    clearTimeout(noteTimer)
    noteTimer = setTimeout(() => (note = ''), 3500)
  }

  async function another() {
    info.tagline = await tagline()
  }

  async function onrun(name, arg) {
    const c = info.commands.find((c) => c.name === name)
    if (!c) return flash(`/${name}: unknown command`)
    if (!c.supported) return flash(`/${name} is not in the graphical UI yet`)
    switch (name) {
      case 'config':
        options = await getOptions()
        query = ''
        sel = 0
        screen = 'config'
        break
      case 'pr':
        startPR(arg)
        return
      case 'open':
      case 'show':
      case 'workdir':
      case 'switch':
      case 'resume':
        startPicker(name, arg)
        return
      case 'reset':
        await reset()
        messages = []
        queue = []
        busy = false
        screen = 'home'
        await another()
        break
      case 'quit':
        quit()
        break
    }
    omni?.focus()
  }

  // What the agent does arrives as events (see ui_chat.go).
  function onagent(e) {
    const last = messages.at(-1)
    switch (e.kind) {
      case 'text-start': messages.push({ kind: 'agent', text: '' }); break
      case 'text': if (last?.kind === 'agent') last.text += e.text; break
      case 'thinking-start': messages.push({ kind: 'thinking', text: '' }); break
      case 'thinking': if (last?.kind === 'thinking') last.text += e.text; break
      case 'tool': messages.push({ kind: 'tool', text: e.text }); break
      case 'tool-error': messages.push({ kind: 'error', text: e.text }); break
      case 'ask': {
        // The tool line becomes the question.
        const m = messages.findLast((m) => m.kind === 'tool' && m.text === e.text)
        if (m) Object.assign(m, { kind: 'ask', id: e.id })
        else messages.push({ kind: 'ask', id: e.id, text: e.text })
        break
      }
      case 'done':
        busy = false
        if (e.isError) messages.push({ kind: 'error', text: e.text })
        else messages.push({ kind: 'note', text: `done · ${e.secs.toFixed(1)}s` })
        sendQueued()
        break
      case 'closed':
        busy = false
        if (e.text) messages.push({ kind: 'error', text: e.text })
        break
    }
  }

  async function dispatch(text) {
    busy = true
    try {
      await send(text)
    } catch (err) {
      busy = false
      messages.push({ kind: 'error', text: String(err) })
    }
  }

  function sendQueued() {
    const next = queue.shift()
    if (!next) return
    messages.push({ kind: 'user', text: next })
    dispatch(next)
  }

  function onsend(text) {
    if (busy) {
      queue.push(text)
      messages.push({ kind: 'note', text: 'queued, sent when the agent finishes: ' + text })
      return
    }
    messages.push({ kind: 'user', text })
    screen = 'chat'
    dispatch(text)
  }

  async function onanswer(m, allow) {
    m.answer = allow ? 'y' : 'n'
    try {
      await answer(m.id, allow)
    } catch (err) {
      messages.push({ kind: 'error', text: String(err) })
    }
  }

  const stop = () => interrupt()

  let listSeq = 0
  async function refresh() {
    const my = ++listSeq
    const p = await list(screen, query)
    if (my !== listSeq || !picking) return
    pick = p
    sel = Math.min(sel, Math.max(p.items.length - 1, 0))
    loadPreview()
  }

  let previewSeq = 0
  async function loadPreview() {
    const my = ++previewSeq
    const it = pick?.items[sel]
    const lines = it ? await preview(screen, it.value) : []
    if (my === previewSeq) previewLines = lines
  }

  function select(i) {
    sel = i
    loadPreview()
  }

  function startPicker(kind, arg) {
    screen = kind
    pick = null
    previewLines = []
    sel = 0
    query = arg
    refresh()
    omni?.setText(arg)
  }

  async function choose(i) {
    const it = pick?.items[i]
    if (!it) return
    if (screen === 'workdir') return chooseWorkdir(it)
    if (screen === 'switch') return chooseBranch(it)
    if (screen === 'resume') return resumeSession(it)
    if (screen === 'show') return
    if (it.dir) return omni?.setText(it.query)
    openedFile = await readFile(it.value)
    cameFrom = screen
    screen = 'file'
  }

  let prSeq = 0
  async function loadPane() {
    const my = ++prSeq
    const r = prRows[sel]
    const v = r ? await loadPRView(r.path) : null
    if (my === prSeq) prPane = v
  }

  async function startPR(arg) {
    screen = 'pr'
    pr = null
    prPane = null
    sel = 0
    query = ''
    omni?.setText('')
    flash('Loading the pull request…')
    try {
      pr = await loadPR(arg)
    } catch (e) {
      flash(String(e))
      return back()
    }
    flash('')
    loadPane()
  }

  async function resumeSession(it) {
    try {
      const past = await resume(it.value)
      messages = [...past.map((m) => ({ kind: m.kind, text: m.text })), { kind: 'note', text: 'resumed · ' + it.meta + ' · the next message continues it' }]
      queue = []
      busy = false
      screen = 'chat'
      omni?.setText('')
    } catch (e) {
      flash(String(e))
    }
  }

  async function chooseWorkdir(it) {
    try {
      const st = await setWorkdir(it.value)
      info.workdir = st.workdir
      info.git = st.git
      branchList = await loadBranches()
      flash('Workdir: ' + st.workdir)
      back()
    } catch (e) {
      flash(String(e))
    }
  }

  async function chooseBranch(it) {
    await onbranch(it.query)
    if (info.git.branch === it.query) back()
  }

  function closeFile() {
    screen = cameFrom
    refresh()
    omni?.focus()
  }

  async function onbranch(name) {
    if (name === info.git.branch) return
    try {
      info.git = await switchBranch(name)
      branchList = await loadBranches()
    } catch (e) {
      flash(String(e))
    }
  }

  function onworkdir() {
    flash('Choosing the working directory is not in the graphical UI yet.')
  }

  async function onset(key, id) {
    try {
      options = await setOption(key, id)
    } catch (e) {
      return flash(String(e))
    }
    if (key === 'theme') applyPalette(await palette(id))
  }

  // The header is the title bar: drag moves the window, a double click zooms it.
  function dragHeader(e) {
    if (e.button !== 0) return
    if (e.detail === 2) window.vZoom?.()
    else window.vDrag?.()
  }

  function back() {
    if (screen === 'file') return closeFile()
    screen = messages.length ? 'chat' : 'home'
    query = ''
    omni?.setText('')
  }

  const rows = $derived(visible(options, query))

  // On the settings screen the omnibox searches, arrows pick a row, ←/→ (with an
  // empty box) and Enter change its value.
  function onkey(e, text) {
    if (screen === 'chat') {
      if (e.key === 'Escape' && text === '' && busy) { stop(); return true }
      return false
    }
    if (screen === 'file') {
      if (e.key === 'Escape' && text === '') { closeFile(); return true }
      return false
    }
    if (screen === 'pr') return onprkey(e, text)
    if (picking) return onpickkey(e, text)
    if (screen !== 'config') return false
    const o = rows[sel]
    const move = (d) => {
      if (!o) return
      const i = o.choices.findIndex((c) => c.id === o.current)
      const n = o.choices[(i + d + o.choices.length) % o.choices.length]
      onset(o.key, n.id)
    }
    switch (e.key) {
      case 'ArrowDown': sel = Math.min(sel + 1, rows.length - 1); return true
      case 'ArrowUp': sel = Math.max(sel - 1, 0); return true
      case 'ArrowRight': if (text === '') { move(1); return true } return false
      case 'ArrowLeft': if (text === '') { move(-1); return true } return false
      case 'Enter': if (!text.startsWith('/')) { move(1); return true } return false
      case 'Escape': if (text === '') { back(); return true } return false
    }
    return false
  }

  // /open and /show: the omnibox filters, arrows pick, → enters or opens, ← goes up.
  function onpickkey(e, text) {
    const n = pick?.items.length ?? 0
    const open = screen === 'open' || screen === 'workdir'
    switch (e.key) {
      case 'ArrowDown': select(Math.min(sel + 1, n - 1)); return true
      case 'ArrowUp': select(Math.max(sel - 1, 0)); return true
      case 'ArrowRight':
        if (open && n && e.target.selectionStart === text.length) {
          const it = pick.items[sel]
          if (screen === 'workdir') { if (it.dir) omni?.setText(it.query) }
          else choose(sel)
          return true
        }
        return false
      case 'ArrowLeft':
        if (open && pick && (text === '' || text.endsWith('/'))) { omni?.setText(pick.up); return true }
        return false
      case 'Enter':
        if (text.startsWith('/')) return false
        if (n) choose(sel)
        return true
      case 'Escape': if (text === '') { back(); return true } return false
    }
    return false
  }

  function onprkey(e, text) {
    switch (e.key) {
      case 'ArrowDown': sel = Math.min(sel + 1, prRows.length - 1); loadPane(); return true
      case 'ArrowUp': sel = Math.max(sel - 1, 0); loadPane(); return true
      case 'Enter': return !text.startsWith('/')
      case 'Escape': if (text === '') { back(); return true } return false
    }
    return false
  }

  function oninput(text) {
    if (screen === 'pr') {
      query = text.startsWith('/') ? '' : text
      sel = 0
      loadPane()
      return
    }
    if (screen !== 'config' && !picking) return
    query = text.startsWith('/') ? '' : text
    sel = 0
    if (picking) refresh()
  }

  function onwindowkey(e) {
    if (e.key === 'Escape' && screen !== 'home' && screen !== 'chat' && e.target.tagName !== 'TEXTAREA') back()
  }
</script>

<svelte:window onkeydown={onwindowkey} />

<main>
  <header onmousedown={dragHeader} role="presentation">
    <span class="brand" class:still={!info?.animate}>Vibing</span>
    {#if title}<span class="crumb"><span class="chev">›</span> {title}</span>{/if}
    <span class="spacer"></span>
    <button class="action" onclick={() => flash('Diff is not in the graphical UI yet.')}>Diff</button>
    <button class="action" onclick={() => flash('Review is not in the graphical UI yet.')}>Review</button>
  </header>

  {#if screen === 'config'}
    <Config {options} {query} {sel} {onset} onback={back} onselect={(i) => (sel = i)} />
  {:else if picking}
    <Picker kind={screen} {pick} {sel} preview={previewLines} empty={screen === 'show' ? 'No commits match' : 'Nothing matches'} onselect={select} onchoose={choose} />
  {:else if screen === 'pr'}
    {#if pr && prPane}
      <PRView {pr} rows={prRows} {sel} view={prPane} onselect={(i) => { sel = i; loadPane() }} />
    {:else}
      <section class="hero"><span class="tagline" style="cursor: default">Loading the pull request…</span></section>
    {/if}
  {:else if screen === 'chat'}
    <Chat {messages} {busy} {onanswer} />
  {:else if screen === 'file' && openedFile}
    <FileView file={openedFile} onclose={closeFile} />
  {:else}
    <section class="hero">
      <div class="logo" role="img" aria-label="v">{@html logo}</div>
      {#if info}
        <button class="tagline" onclick={another} title="Another one">{info.tagline}</button>
      {/if}
    </section>
  {/if}

  <Omnibox bind:this={omni} commands={info?.commands ?? []} {options} {onset} {note}
    workdir={info?.workdir ?? ''} git={info?.git} branches={branchList} {onbranch} {onworkdir}
    placeholder={placeholder}
    {busy} onstop={stop} {onrun} {onsend} {oninput} {onkey} />

</main>

<style>
  header {
    flex: none;
    height: calc(var(--bar-cy, 19px) * 2); /* its middle is the window buttons' middle */
    margin: 0 -24px;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 16px 0 calc(var(--bar-left, 70px) + 14px); /* clear of the window buttons */
  }
  .spacer { flex: 1; }
  .action {
    background: none;
    border: 0;
    border-radius: 6px;
    padding: 3px 10px;
    color: var(--dim);
    font-size: 12px;
    cursor: pointer;
  }
  .action:hover { color: var(--text); background: var(--raised); }
  .crumb { color: var(--text); font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .chev { color: var(--dim); margin: 0 2px; }

  /* The brand shimmers: the logo's gradient drifts through the letters. */
  .brand {
    font-size: 13px;
    line-height: 1;
    padding: 0.1em 0 0.3em; /* the gradient is clipped to the box: leave room for the g's tail */
    margin: -0.1em 0 -0.3em;
    font-weight: 600;
    letter-spacing: 0.16em;
    text-transform: none;
    background: linear-gradient(90deg, var(--accent), var(--accent2), var(--accent));
    background-size: 200% 100%;
    -webkit-background-clip: text;
    background-clip: text;
    -webkit-text-fill-color: transparent;
    animation: shimmer 6s linear infinite;
  }
  .brand.still { animation: none; background-size: 100% 100%; }
  @media (prefers-reduced-motion: reduce) {
    .brand { animation: none; }
  }
  @keyframes shimmer {
    to { background-position: -200% 0; }
  }

  main {
    height: 100%;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    padding: 0 24px 16px;
  }

  .hero {
    flex: 1;
    min-height: 0;
    overflow: hidden; /* never runs under the omnibox */
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 14px;
  }
  .logo { width: 96px; height: 96px; flex: none; }
  @media (max-height: 520px) {
    .logo { width: 56px; height: 56px; }
  }
  @media (max-height: 400px) {
    .hero { visibility: hidden; }
  }
  .logo :global(svg) { width: 100%; height: 100%; }

  .tagline {
    background: none;
    border: 0;
    padding: 2px 8px;
    border-radius: 6px;
    color: var(--dim);
    cursor: pointer;
  }
  .tagline:hover { color: var(--text); }
</style>
