<script>
  import Text from './Text.svelte'

  let { messages, busy, onanswer, level = $bindable(2) } = $props()

  let box
  let stick = true // follow the newest text unless the reader scrolled up

  function onscroll() {
    stick = box.scrollHeight - box.scrollTop - box.clientHeight < 40
  }

  $effect(() => {
    messages.length
    messages.at(-1)?.text
    messages.at(-1)?.out
    busy
    if (stick && box) queueMicrotask(() => (box.scrollTop = box.scrollHeight))
  })

  // "Edit src/a.go" -> the part after the tool's name.
  const argOf = (m) => (m.tool && m.text.startsWith(m.tool) ? m.text.slice(m.tool.length).trim() : m.text)
  const isDiff = (m) => m.tool === 'Edit' || m.tool === 'MultiEdit'
  const lineKind = (l) => (l.startsWith('+ ') ? 'add' : l.startsWith('- ') ? 'del' : '')
  const words = (t) => t.trim().split(/\s+/).filter(Boolean).length
  const lastLine = (t) => t.trim().split('\n').at(-1) ?? ''
  // A panel is open when the reader opened it, or while it waits for an answer.
  // The panels are always open: what a tool was given only when it says more than the header
  // (an edit, a file written, a multi-line command, or the thing being asked about).
  const showDetail = (m) =>
    m.detail && (isDiff(m) || m.tool === 'Write' || (m.tool === 'Bash' && m.detail.includes('\n')) || (m.kind === 'ask' && !m.answer))

  // How much of what the agent did is shown, from the reply alone to everything:
  // 0 reply · 1 + edits · 2 + commands and other tools · 3 + reading, searching, reasoning.
  // Errors and questions to the reader show at every level. What is left out becomes one line.
  const LEVELS = ['Reply', 'Edits', 'Actions', 'Everything']
  const LOOK = new Set(['Read', 'Grep', 'Glob', 'LS', 'NotebookRead', 'TodoWrite'])
  const EDIT = new Set(['Edit', 'MultiEdit', 'Write', 'NotebookEdit'])
  const tier = (m) => (LOOK.has(m.tool) ? 3 : EDIT.has(m.tool) ? 1 : 2) // the lowest level that shows it
  const hidden = (m) => m.kind === 'tool' && m.state !== 'error' && tier(m) > level
  const WHAT = [
    [(m) => m.tool === 'Read', 'read', 'file'],
    [(m) => LOOK.has(m.tool), 'looked', 'time'],
    [(m) => EDIT.has(m.tool), 'edited', 'file'],
    [(m) => m.tool === 'Bash', 'ran', 'command'],
    [() => true, 'made', 'other call'],
  ]
  const summary = (ms) => {
    const n = WHAT.map(() => 0)
    for (const m of ms) n[WHAT.findIndex(([f]) => f(m))]++
    return WHAT.map(([, verb, noun], i) => (n[i] ? `${verb} ${n[i]} ${noun}${n[i] > 1 ? 's' : ''}` : '')).filter(Boolean).join(' · ')
  }
  let opened = $state({}) // summaries the reader opened, by the index of their first call

  // Thinking is not a row of its own: it belongs to what follows it (a tool call or a reply)
  // and opens from the icon of that panel. Only thinking that has nothing after it yet stays
  // as a row, so that you can see the agent is on it.
  const items = $derived.by(() => {
    const out = []
    let pending = null
    let run = null // the calls left out since the last row shown
    const flush = () => {
      if (run) out.push({ m: { kind: 'sum', ms: run, key: messages.indexOf(run[0]) }, t: null })
      run = null
    }
    for (const m of messages) {
      if (m.kind === 'thinking') {
        pending = m
        continue
      }
      if (m.kind === 'user') pending = null
      if (hidden(m)) {
        ;(run ??= []).push(m)
        continue
      }
      flush()
      const owner = m.kind === 'tool' || m.kind === 'ask' || m.kind === 'agent'
      out.push({ m, t: owner && level === 3 ? pending : null })
      if (owner) pending = null
    }
    flush()
    if (pending && level === 3) out.push({ m: pending, t: null, tail: true })
    return out
  })

  // One card per question: the question on top, everything the agent did and said under it.
  // A command of the user's own stands alone and ends the card (what comes after starts a new one).
  const groups = $derived.by(() => {
    const out = []
    let turn = null
    for (const it of items) {
      if (it.m.kind === 'shell') {
        out.push({ shell: it.m })
        turn = null
      } else if (it.m.kind === 'user') {
        turn = { q: it.m, kids: [] }
        out.push(turn)
      } else {
        if (!turn) {
          turn = { q: null, kids: [] }
          out.push(turn)
        }
        turn.kids.push(it)
      }
    }
    return out
  })
  const waiting = (g) => g.kids.some((c) => c.m.kind === 'ask' && !c.m.answer)
  // Long output is cut to its first lines with a button for the rest.
  const cut = (text, m) => {
    const lines = text.replace(/\n$/, '').split('\n')
    const LINES = m.tool === 'Bash' || m.kind === 'shell' ? 30 : 12 // commands show more than the rest
    return m.all || lines.length <= LINES ? { lines, rest: 0 } : { lines: lines.slice(0, LINES), rest: lines.length - LINES }
  }
  // The reasoning shows at the last level only: three lines, and "Expand" for all of it.
  const view = (t) => (level >= 3 ? (t.view ?? 'peek') : '')
</script>

<section class="chat" bind:this={box} {onscroll}>
  <div class="col">
    {#each groups as g, gi (gi)}
      {#if g.shell}
        {@const m = g.shell}
        <div class="blk">
          <div class="bh">
            <div class="hmain">
              <span class="ps sh">$</span><span class="arg">{m.cmd}</span>
              {#if m.code === null}<span class="st run">running</span>
              {:else if m.code === 0}<span class="st ok">✓</span>
              {:else if m.code === -1}<span class="st bad">stopped</span>
              {:else}<span class="st bad">exit {m.code}</span>{/if}
            </div>
          </div>
          {#if m.out}
            {@const o = cut(m.out, m)}
            <div class="bb"><pre class="code out">{o.lines.join('\n')}</pre>{#if o.rest}{@render more(m, o.rest)}{/if}</div>
          {/if}
        </div>
      {:else}
        <div class="blk turn" class:ask={waiting(g)}>
          {#if g.q}
            <div class="q"><span class="ps">*</span><span class="qt">{g.q.text}</span></div>
          {/if}
          {#each g.kids as it, ki (ki)}
            {@const { m, t, tail } = it}
            {#if m.kind === 'agent'}
              <div class="part" class:first={ki === 0 && g.q}>
                <Text text={m.text} />
                {#if t && view(t)}{@render thought(t)}{/if}
              </div>
            {:else if m.kind === 'thinking' && tail}
              <div class="part live">
                <div class="livebtn">
                  <span class="arg dim">Thinking{m.text ? ': ' + lastLine(m.text) : '…'}</span><span class="st run">thinking</span>
                </div>
                {#if view(m)}{@render thought(m)}{/if}
              </div>
            {:else if m.kind === 'tool' || m.kind === 'ask'}
              <div class="sub" class:error={m.state === 'error'}>
                <div class="bh">
                  <div class="hmain">
                    {#if m.tool === 'Bash'}
                      <span class="ps">$</span><span class="arg">{argOf(m)}</span>
                    {:else}
                      <span class="tn">{m.tool || 'tool'}</span><span class="arg">{argOf(m)}</span>
                    {/if}
                    {#if m.kind === 'ask' && m.answer}
                      <span class="st" class:ok={m.answer === 'y'} class:bad={m.answer !== 'y'}>{m.answer === 'y' ? 'allowed' : 'denied'}</span>
                    {/if}
                    {#if m.kind === 'ask' && !m.answer}<span class="st run">waiting for you</span>
                    {:else if m.state === 'run'}<span class="st run">running</span>
                    {:else if m.state === 'error'}<span class="st bad">error</span>
                    {:else if m.state === 'ok'}<span class="st ok">✓</span>{/if}
                  </div>
                </div>
                {#if t && view(t)}{@render thought(t)}{/if}
                {#if showDetail(m) || m.out}
                  <div class="bb">
                    {#if showDetail(m)}
                      {@const d = cut(m.detail, m)}
                      <pre class="code">{#each d.lines as l}<span class={isDiff(m) ? lineKind(l) : ''}>{l || ' '}</span>{/each}</pre>
                      {#if d.rest}{@render more(m, d.rest)}{/if}
                    {/if}
                    {#if m.out}
                      {#if showDetail(m) || m.state === 'error'}<div class="lab">{m.state === 'error' ? 'error' : 'output'}</div>{/if}
                      {@const o = cut(m.out, m)}
                      <pre class="code out">{o.lines.join('\n')}</pre>
                      {#if o.rest}{@render more(m, o.rest)}{/if}
                    {/if}
                  </div>
                {/if}
                {#if m.kind === 'ask' && !m.answer}
                  <div class="actions">
                    <span class="hint">Allow this?</span>
                    <button class="yes" onclick={() => onanswer(m, true)}>Allow</button>
                    <button class="no" onclick={() => onanswer(m, false)}>Deny</button>
                  </div>
                {/if}
              </div>
            {:else if m.kind === 'sum'}
              <div class="sub quiet">
                <button class="sumbtn" onclick={() => (opened[m.key] = !opened[m.key])} aria-expanded={!!opened[m.key]}>
                  <span class="car" class:open={opened[m.key]}>▸</span><span class="arg dim">{summary(m.ms)}</span>
                </button>
                {#if opened[m.key]}
                  <div class="bb">
                    {#each m.ms as c}<div class="lookrow"><span class="tn">{c.tool}</span><span class="arg">{argOf(c)}</span></div>{/each}
                  </div>
                {/if}
              </div>
            {:else if m.kind === 'error'}
              <div class="part error">{m.text}</div>
            {:else}
              <div class="part note">{m.text}</div>
            {/if}
          {/each}
          {#if busy && gi === groups.length - 1 && !waiting(g)}
            <div class="part"><div class="working"><span class="dot"></span><span class="dot"></span><span class="dot"></span></div></div>
          {/if}
        </div>
      {/if}
    {/each}
  </div>
</section>

{#snippet more(m, n)}
  <button class="showall" onclick={() => (m.all = true)}>Show all · {n} more lines</button>
{/snippet}


{#snippet thought(t)}
  <div class="thought" class:full={view(t) === 'full'}>
    <div class="prose">{t.text}</div>
    <button class="more" onclick={() => (t.view = view(t) === 'full' ? 'peek' : 'full')}>{view(t) === 'full' ? 'Collapse' : 'Expand'}</button>
  </div>
{/snippet}

<style>
  .chat { flex: 1; min-height: 0; overflow-y: auto; padding: 8px 0 14px; user-select: text; scrollbar-width: none; }
  .chat::-webkit-scrollbar { display: none; } /* the chat still scrolls; the bar is hidden */
  .sumbtn { display: flex; align-items: baseline; gap: 8px; width: 100%; padding: 4px 12px 4px 8px; background: none; border: 0; text-align: left; cursor: pointer; }
  .sumbtn:hover { background: var(--raised); }
  .lookrow { display: flex; gap: 8px; padding: 2px 12px 2px 26px; font: 12px var(--mono); color: var(--dim); }
  .col { width: 100%; max-width: var(--omni); margin: 0 auto; display: flex; flex-direction: column; gap: 10px; }

  .body { padding: 8px 12px 10px; color: var(--text); line-height: 1.6; user-select: text; }
  .body.plain { white-space: pre-wrap; color: var(--strong); }
  .tn.who { color: var(--accent2); }
  .tn.who-agent { color: var(--dim); font-weight: 500; }
  .note { color: var(--dim); font-size: 12px; }
  .error { color: var(--c-deleted, #e06c75); }

  /* One panel for everything the agent or the shell did: a header, and a body that opens. */
  .blk { border: 1px solid var(--line); border-radius: 8px; background: var(--panel); overflow: hidden; }
  .blk.error { border-color: color-mix(in srgb, var(--c-deleted, #f85149) 45%, var(--line)); }
  .blk.ask { border-color: var(--accent); background: var(--accent-soft); }
  /* The header of a panel. */
  .bh { display: flex; align-items: center; }
  .bh > .hmain { flex: 1; min-width: 0; }
  .hmain {
    display: flex;
    align-items: baseline;
    gap: 8px;
    width: 100%;
    padding: 6px 12px 6px 8px;
    background: none;
    border: 0;
    text-align: left;
    cursor: pointer;
    font: 12.5px var(--mono);
    color: var(--strong);
  }
  .hmain { cursor: default; }


  /* The reasoning: three lines at first, the whole thing (scrolling) when expanded. */
  .thought { margin: -4px 0 0; padding: 8px 12px 6px; border: 1px dashed var(--line); border-radius: 8px; color: var(--dim); }
  .thought .prose { padding: 0; font-size: 12.5px; line-height: 1.55; white-space: pre-wrap; max-height: 4.7em; overflow: hidden; -webkit-mask-image: linear-gradient(#000 60%, transparent); mask-image: linear-gradient(#000 60%, transparent); }
  .thought.full .prose { max-height: none; overflow: visible; -webkit-mask-image: none; mask-image: none; }
  .showall { display: block; width: 100%; padding: 5px 12px; background: var(--raised); border: 0; border-top: 1px solid var(--line); color: var(--accent); font-size: 12px; text-align: left; cursor: pointer; }
  .showall:hover { background: var(--accent-soft); }
  .more { margin-top: 4px; padding: 0; background: none; border: 0; color: var(--accent2); font-size: 12px; cursor: pointer; }
  .blk.live { background: none; border-style: dashed; }
  .blk .sub .thought { margin: 0; border: 0; border-top: 1px dashed var(--line); border-radius: 0; }
  .car { flex: none; width: 10px; color: var(--dim); transition: transform 0.12s; text-align: center; }
  .car.open { transform: rotate(90deg); }
  .tn { flex: none; color: var(--accent); font-weight: 600; }
  .arg { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .arg.dim { color: var(--dim); font-size: 12px; font-family: 'Inter', sans-serif; font-style: italic; }
  .ps { color: var(--accent); user-select: none; flex: none; }
  .ps.sh { color: var(--accent2); }
  .st { flex: none; font: 11.5px 'Inter', sans-serif; }
  .st.ok { color: var(--c-added, #3fb950); }
  .st.bad { color: var(--c-deleted, #f85149); }
  .st.run { color: var(--accent2); animation: blink 1.2s ease-in-out infinite; }
  @keyframes blink { 50% { opacity: 0.35; } }

  .bb { border-top: 1px solid var(--line); }
  .lab { padding: 4px 12px 0; color: var(--dim); font-size: 11px; text-transform: uppercase; letter-spacing: 0.06em; }
  pre.code { margin: 0; padding: 8px 12px; font: 12px/1.5 var(--mono); color: var(--text); white-space: pre-wrap; overflow-wrap: anywhere; user-select: text; }
  pre.code span { display: block; }
  pre.code span.add { background: color-mix(in srgb, var(--c-added, #3fb950) 14%, transparent); color: var(--c-added, #3fb950); }
  pre.code span.del { background: color-mix(in srgb, var(--c-deleted, #f85149) 14%, transparent); color: var(--c-deleted, #f85149); }
  pre.code.out { color: var(--text); }
  .bb pre.code + .lab { border-top: 1px solid var(--line); margin-top: 0; padding-top: 6px; }

  .actions { display: flex; align-items: center; gap: 8px; padding: 6px 12px 8px; border-top: 1px solid var(--divider); }
  .hint { flex: 1; color: var(--dim); font-size: 12px; }
  .actions button { border: 0; border-radius: 6px; padding: 3px 12px; cursor: pointer; font-size: 12px; }
  .actions .yes { background: var(--accent); color: var(--bg); }
  .actions .no { background: var(--raised); color: var(--text); }

  .working { display: flex; gap: 5px; padding: 2px 0; }
  .dot { width: 6px; height: 6px; border-radius: 50%; background: var(--dim); animation: pulse 1.2s ease-in-out infinite; }
  .dot:nth-child(2) { animation-delay: 0.2s; }
  .dot:nth-child(3) { animation-delay: 0.4s; }
  @keyframes pulse { 0%, 80%, 100% { opacity: 0.25; } 40% { opacity: 1; } }
  @media (prefers-reduced-motion: reduce) { .dot, .st.run { animation: none; opacity: 0.6; } }

  /* A question and its answer are one card: the question on top, the answer under it in parts. */
  .blk.turn.ask { background: var(--panel); } /* waiting for an answer: only the border says so */
  .turn .q { display: flex; align-items: baseline; gap: 8px; padding: 8px 12px; color: var(--strong); line-height: 1.5; }
  .turn .q .ps { font: 600 14px var(--mono); }
  .turn .qt { flex: 1; min-width: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
  .turn .part { position: relative; padding: 8px 14px 10px; color: var(--text); line-height: 1.6; user-select: text; }
  .turn .part.first, .turn .sub, .turn .q + .part { border-top: 1px solid var(--line); }
  .turn .part.note { padding-top: 4px; padding-bottom: 6px; color: var(--dim); font-size: 12px; }
  .turn .part.error { color: var(--c-deleted, #e06c75); }
  .turn .part.live { padding: 0; }
  .livebtn { display: flex; align-items: baseline; gap: 8px; width: 100%; padding: 6px 12px; background: none; border: 0; text-align: left; cursor: pointer; font: inherit; }
  .livebtn:disabled { cursor: default; }
  .turn .sub { background: none; }
  .turn .sub.error { background: color-mix(in srgb, var(--c-deleted, #f85149) 6%, transparent); }
  .turn .part .thought { margin: 8px 0 0; }
  .turn .part::after { content: ''; display: block; clear: both; }
</style>
