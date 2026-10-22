<script>
  import { onMount, untrack } from 'svelte'
  import { start, tagline, getOptions, setOption, palette, git, branches as loadBranches, switchBranch, list, preview, file as readFile, setWorkdir, loadPR, branchPRs, onAgent, send, shell, shellStop, answer, interrupt, reset, resume, quit } from './api.js'
  import { applyPalette } from './theme.js'
  import Omnibox from './Omnibox.svelte'
  import Config from './Config.svelte'
  import Picker from './Picker.svelte'
  import { kindOf, toggleRender } from './prefs.svelte.js'
  import FileView from './FileView.svelte'
  import Chat from './Chat.svelte'
  import PRView from './PRView.svelte'
  import { visible, fuzzy } from './filter.js'
  import Logo from './Logo.svelte'

  let info = $state(null)
  let options = $state([])
  let screen = $state('home')
  let note = $state('')
  let omni
  let branchList = $state([])
  let branchPrs = $state([]) // the PRs of the current branch, for the chip beside it
  let query = $state('')
  let sel = $state(0)
  let pick = $state(null) // the list of /open and /show
  let previewLines = $state([])
  let openedFile = $state(null)
  // How the omnibox searches inside an open file: a regular expression, or fuzzy (the letters in order).
  let findMode = $state('regexp')
  // The search in an open file has its choice in the omnibox, where the agent's settings are on the other screens.
  const searchOption = $derived(
    screen === 'file' && openedFile?.edit
      ? { key: 'find', label: 'Search', current: findMode, choices: [{ id: 'regexp', name: 'regexp' }, { id: 'fuzzy', name: 'fuzzy' }] }
      : null,
  )
  let fileview = $state() // the open file's view, to search in it
  let fileQuery = $state('')
  let fileDirty = $state(false) // the open file has changes that are not saved
  let pr = $state(null) // the open pull request
  let prview = $state() // the PR screen, to hand it keys
  let messages = $state([]) // the conversation with the agent
  let busy = $state(false)
  let queue = [] // prompts typed while the agent works
  let cameFrom = 'home' // the screen to return to from a file

  const placeholders = {
    file: 'Search in the file · Enter next · Tab: regexp / fuzzy · Esc close',
    pr: 'Filter files or threads · ↑↓ pick · ←→ main / files / threads / CI · Esc back',
    chat: 'Message the agent, or type / for commands · Esc stops a running turn',
    config: 'Search settings · ↑↓ pick · ←→ change · / for commands',
    workdir: 'Type to find a folder · ↑↓ pick · → enter · ← up · Enter choose · Esc back',
    switch: 'Type to find a branch · ↑↓ pick · Enter switch · Esc back',
    resume: 'Type to find a session · ↑↓ pick · Esc back',
    open: 'Type to find a file · ↑↓ pick · → enter or open · ← up · Esc back',
    show: 'Type to filter commits · ↑↓ pick · Esc back',
  }
  const placeholder = $derived(placeholders[screen] ?? 'Ask the agent, or type / for commands')
  const picking = $derived(['open', 'log', 'workdir', 'branch', 'resume'].includes(screen))

  onMount(async () => {
    onAgent(onagent)
    info = await start()
    applyPalette(info.palette)
    options = await getOptions()
    level = Math.max(savedDetail(), 0)
    branchList = await loadBranches()
    omni?.focus()
    // In a plain browser (npm run dev) #/open, #/config and so on jump to a screen.
    if (!window.vStart && location.hash.startsWith('#/')) {
      const [name, ...rest] = decodeURIComponent(location.hash.slice(2)).split(' ')
      if (name === 'prfile' || name === 'prsplit') {
        if (name === 'prsplit') localStorage.setItem('v.prdiff', 'split')
        await startPR('')
        setTimeout(() => prview?.key({ key: 'ArrowRight' }, ''), 100)
        for (let i = 0; i < 4; i++) setTimeout(() => prview?.key({ key: 'ArrowDown' }, ''), 200 + i * 50)
      } else if (['openmd', 'opensvg', 'openpng'].includes(name)) {
        onrun('open', '')
        setTimeout(() => select({ openmd: 1, opensvg: 2, openpng: 3 }[name]), 300)
      } else if (name === 'openfile') {
        onrun('open', '')
        setTimeout(() => choose(4), 300)
        setTimeout(() => omni?.setText('mn'), 700)
        setTimeout(() => (findMode = 'fuzzy'), 600)
      } else if (name === 'prci') {
        await startPR('')
        setTimeout(() => prview?.showTab('ci'), 150)
      } else if (name === 'shell') {
        mode = 'shell'
        onshell(rest.join(' '))
      } else if (name === 'light') onset('theme', 'light')
      else if (name === 'send') onsend(rest.join(' '))
      else onrun(name, rest.join(' '))
    }
    // The branch and the changed files move while the agent works.
    const timer = setInterval(async () => {
      info.git = await git()
      branchList = await loadBranches()
    }, 4000)
    return () => clearInterval(timer)
  })

  // A command may have changed the branch or the files.
  async function refreshGit() {
    info.git = await git()
    branchList = await loadBranches()
  }

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
      case 'log':
      case 'workdir':
      case 'branch':
      case 'resume':
        startPicker(name, arg)
        return
      case 'reset':
        await reset()
        messages = []
        sentUpTo = 0
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
      case 'tool': messages.push({ kind: 'tool', text: e.text, tool: e.tool, toolId: e.toolId, detail: e.detail, out: '', state: 'run' }); break
      case 'tool-result': {
        // The result lands in the panel of its call (which may have turned into a question).
        const m = messages.findLast((m) => m.toolId && m.toolId === e.toolId)
        if (m) Object.assign(m, { out: e.text, state: e.isError ? 'error' : 'ok' })
        break
      }
      case 'tool-error': break // the panel of the call shows it
      case 'note': messages.push({ kind: 'note', text: e.text }); break
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
      case 'shell-out': {
        const m = messages.findLast((m) => m.id === e.id)
        if (m) m.out += e.text
        break
      }
      case 'shell-done': {
        const m = messages.findLast((m) => m.id === e.id)
        if (m) {
          m.code = e.turns ?? 0
          if (e.text) m.out += e.text + '\n'
        }
        shellId = ''
        refreshGit()
        break
      }
      case 'closed':
        busy = false
        if (e.text) messages.push({ kind: 'error', text: e.text })
        break
    }
  }

  async function dispatch(text) {
    busy = true
    try {
      await send(text, shellContext())
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

  // The shell mode: the omnibox runs its text as a command; the output lands in the
  // chat, and the agent gets it with the next message.
  // How much of the agent's work the chat shows. It is a setting (Chat detail in /config): the
  // slider above the input and the setting are the same value.
  const DETAIL = ['answer', 'edit', 'commands', 'thinking']
  let level = $state(2)
  const savedDetail = () => DETAIL.indexOf(options.find((o) => o.key === 'detail')?.current ?? 'commands')
  $effect(() => {
    const n = level
    untrack(() => {
      if (options.length && n !== savedDetail()) setOption('detail', DETAIL[n]).then((o) => (options = o)).catch(() => {})
    })
  })

  let mode = $state('agent')
  let shellId = $state('') // the running command
  let sentUpTo = 0 // messages before this index were already shown to the agent
  $effect(() => {
    if (screen !== 'home' && screen !== 'chat') mode = 'agent'
  })

  async function onshell(cmd) {
    if (shellId) return flash('A command is still running: Ctrl+C stops it')
    screen = 'chat'
    const cd = cmd.match(/^cd(?:\s+(.*))?$/)
    if (cd) {
      // A directory change has to happen here: a child process cannot change ours.
      const m = { kind: 'shell', id: '', cmd, out: '', code: 0 }
      messages.push(m)
      try {
        const st = await setWorkdir((cd[1] ?? '~').trim().replace(/^["']|["']$/g, ''))
        info.workdir = st.workdir
        info.git = st.git
        branchList = await loadBranches()
        m.out = st.workdir + '\n'
      } catch (err) {
        m.code = 1
        m.out = String(err).replace(/^Error: /, '') + '\n'
      }
      return
    }
    const id = 's' + Date.now()
    messages.push({ kind: 'shell', id, cmd, out: '', code: null })
    shellId = id
    try {
      await shell(id, cmd, outputColumns())
    } catch (err) {
      const m = messages.findLast((m) => m.id === id)
      if (m) Object.assign(m, { code: 1, out: String(err) })
      shellId = ''
    }
  }

  // How many characters fit in a line of a command's output: the omnibox's width less the
  // block's padding, over the width of one character of the output font (12 px mono).
  function outputColumns() {
    const probe = document.createElement('span')
    probe.style.cssText = 'position:absolute;visibility:hidden;white-space:pre;font:12px var(--mono)'
    probe.textContent = 'M'.repeat(100)
    document.body.appendChild(probe)
    const ch = probe.getBoundingClientRect().width / 100
    probe.remove()
    const width = (document.querySelector('.wrap')?.clientWidth ?? 900) - 26
    return ch > 0 ? Math.max(20, Math.floor(width / ch)) : 100
  }

  // What the shell mode did since the last message to the agent, as a prefix for the next one.
  function shellContext() {
    const ran = messages.slice(sentUpTo).filter((m) => m.kind === 'shell' && m.code !== null)
    sentUpTo = messages.length
    if (!ran.length) return ''
    let out = ''
    for (const m of ran) {
      const tail = m.out.length > 3000 ? '… ' + m.out.slice(-3000) : m.out
      out += `$ ${m.cmd}\n${tail}${tail.endsWith('\n') || !tail ? '' : '\n'}[exit ${m.code}]\n\n`
    }
    return 'The user ran these shell commands in the working directory since their last message (output may be cut):\n\n' + out.slice(-12000).trimEnd()
  }

  const stop = () => (shellId ? shellStop(shellId) : interrupt())

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
    if (screen === 'branch') return chooseBranch(it)
    if (screen === 'resume') return resumeSession(it)
    if (screen === 'log') return
    if (it.dir) return omni?.setText(it.query)
    openedFile = await readFile(it.value)
    fileQuery = ''
    omni?.setText('')
    cameFrom = screen
    screen = 'file'
  }

  async function startPR(arg) {
    screen = 'pr'
    pr = null
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
  }

  async function resumeSession(it) {
    try {
      const past = await resume(it.value)
      messages = [...past.map((m) => ({ kind: m.kind, text: m.text })), { kind: 'note', text: 'resumed · ' + it.meta + ' · the next message continues it' }]
      sentUpTo = messages.length
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

  // Esc always leaves the file. With changes that are not saved, the first Esc says so and a second one (within
  // a few seconds) leaves anyway.
  let discardAt = 0
  function leaveFile() {
    if (fileDirty && Date.now() - discardAt > 4000) {
      discardAt = Date.now()
      return flash('Unsaved changes: save them, or press Esc again to discard them')
    }
    closeFile(true)
  }

  function closeFile(force = false) {
    omni?.setText('')
    if (fileDirty && !force) return flash('Unsaved changes: save them, or press Esc again to discard them')
    fileDirty = false
    screen = cameFrom
    refresh()
    omni?.focus()
  }

  // GitHub is slow to answer, so the chip fills in when it can; a branch or folder change asks again.
  let prsFor = ''
  async function loadBranchPrs() {
    const key = (info?.workdir ?? '') + '@' + (info?.git?.branch ?? '')
    if (!info?.git?.branch || key === prsFor) return
    prsFor = key
    branchPrs = []
    try {
      const found = await branchPRs()
      if (key === prsFor) branchPrs = found
    } catch {}
  }
  $effect(() => {
    info?.workdir
    info?.git?.branch
    untrack(loadBranchPrs)
  })

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
    if (key === 'detail') level = Math.max(DETAIL.indexOf(id), 0)
  }

  // The header is the title bar: drag moves the window, a double click zooms it.
  function dragHeader(e) {
    if (e.button !== 0) return
    if (e.detail === 2) window.vZoom?.()
    else window.vDrag?.()
  }

  function back() {
    if (screen === 'file') return leaveFile()
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
      if (e.key === 'Escape') { leaveFile(); return true }
      if (e.key === 'Tab') { findMode = findMode === 'regexp' ? 'fuzzy' : 'regexp'; return true }
      // The omnibox searches the file: Enter or the arrows go from match to match.
      if (!text.startsWith('/') && (e.key === 'Enter' || e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
        if (text) fileview?.find(e.key === 'ArrowUp' || (e.key === 'Enter' && e.shiftKey) ? -1 : 1)
        return true
      }
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
    if (prview?.key(e, text)) return true
    if (e.key === 'Escape' && text === '') {
      back()
      return true
    }
    return false
  }

  function oninput(text) {
    if (screen === 'pr') {
      query = text.startsWith('/') ? '' : text
      return
    }
    if (screen === 'file') {
      fileQuery = text
      return
    }
    if (screen !== 'config' && !picking) return
    query = text.startsWith('/') ? '' : text
    sel = 0
    if (picking) refresh()
  }

  // The keys (arrows, Enter, typing) go through the omnibox, so a click on a
  // button must not leave the focus on it; a text selection is left alone.
  function refocus(e) {
    if (e.target.closest?.('textarea, input, a') || String(getSelection())) return
    omni?.focus()
  }

  function onwindowkey(e) {
    // Alt+R: the open or selected file raw <-> rendered (Markdown, SVG, a picture).
    if (e.altKey && e.code === 'KeyR' && (screen === 'open' || screen === 'file')) {
      e.preventDefault()
      const p = screen === 'file' ? openedFile?.path : pick?.items?.[sel]?.value
      if (p) toggleRender(p)
      return
    }
    if (e.key === 'Escape' && screen !== 'home' && screen !== 'chat' && e.target.tagName !== 'TEXTAREA') back()
  }
</script>

<svelte:window onkeydown={onwindowkey} onclick={refocus} />

<main>
  <header onmousedown={dragHeader} role="presentation">
    <span class="brand" class:still={!info?.animate}>Vibing…</span>
  </header>

  {#if screen === 'config'}
    <Config {options} {query} {sel} {onset} onback={back} onselect={(i) => (sel = i)} />
  {:else if picking}
    <Picker kind={screen} onjump={(q) => omni?.setText(q)} {pick} {sel} preview={previewLines} empty={screen === 'log' ? 'No commits match' : 'Nothing matches'} onselect={select} onchoose={choose} />
  {:else if screen === 'pr'}
    {#if pr}
      <PRView bind:this={prview} bind:pr {query} />
    {:else}
      <section class="hero"><span class="tagline" style="cursor: default">Loading the pull request…</span></section>
    {/if}
  {:else if screen === 'chat'}
    <Chat {messages} {busy} {onanswer} bind:level />
  {:else if screen === 'file' && openedFile}
    <FileView bind:this={fileview} query={fileQuery} bind:mode={findMode} file={openedFile} bind:dirty={fileDirty} onclose={() => closeFile(true)} onescape={leaveFile} />
  {:else}
    <section class="hero">
      <Logo animate={!!info?.animate} />
      {#if info}
        <button class="tagline" onclick={another} title="Another one">// {info.tagline}</button>
      {/if}
    </section>
  {/if}

  <Omnibox bind:this={omni} {searchOption} onsearch={(_, id) => (findMode = id)} commands={info?.commands ?? []} {options} {onset} {note}
    workdir={info?.workdir ?? ''} git={info?.git} branches={branchList} prs={branchPrs} {onbranch} {onworkdir}
    placeholder={placeholder}
    filtering={screen !== 'home' && screen !== 'chat'} bind:level bind:mode busy={mode === 'shell' ? !!shellId : busy} onstop={stop} {onshell} {onrun} {onsend} {oninput} {onkey} />

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

  /* The brand shimmers: the logo's gradient drifts through the letters. */
  .brand {
    font-size: 13px;
    line-height: 1;
    padding: 0.1em 0 0.3em; /* the gradient is clipped to the box: leave room for the g's tail */
    margin: -0.1em 0 -0.3em;
    font-weight: 400;
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
  @media (max-height: 400px) {
    .hero { visibility: hidden; }
  }

  .tagline {
    background: none;
    border: 0;
    padding: 2px 8px;
    border-radius: 6px;
    color: var(--c-comment);
    font: italic 13px var(--mono);
    cursor: pointer;
  }
  .tagline:hover { color: var(--text); }
</style>
