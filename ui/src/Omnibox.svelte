<script>
  import { untrack } from 'svelte'
  import Dropdown from './Dropdown.svelte'

  // The input at the bottom: plain text goes to the agent, "/" opens command suggestions.
  let { filtering = false, searchOption = null, onsearch, level = $bindable(2), mode = $bindable('agent'), commands, options, onset, note, busy = false, onstop, onshell, workdir, git, branches, prs = [], onbranch, onworkdir, placeholder, onrun, onsend, oninput, onkey: onextra } = $props()

  let text = $state('')
  let sel = $state(0)
  const shellMode = $derived(mode === 'shell')
  let history = [] // shell commands typed in this window, newest last
  let at = -1 // position in history while browsing it with the arrows
  let box
  const branchOption = $derived(
    git?.branch ? { key: 'branch', label: 'Branch', current: git.branch, choices: branches.map((b) => ({ id: b, name: b })) } : null,
  )
  // Several PRs on one branch: pick which to open.
  const prOption = $derived({ key: 'pr', label: 'Pull requests', current: '', choices: prs.map((p) => ({ id: p.url, name: `#${p.number} ${p.title}` })) })
  const dropdowns = $derived(['model', 'effort', 'mode'].map((k) => options.find((o) => o.key === k)).filter(Boolean))

  export function setText(t) {
    text = t
    box?.focus()
  }

  export function focus() {
    box?.focus()
  }

  // Suggestions show while a command name is being typed (no space yet).
  const suggestions = $derived.by(() => {
    if (shellMode || !text.startsWith('/') || /\s/.test(text)) return []
    const p = text.slice(1).toLowerCase()
    return commands.filter((c) => c.name.startsWith(p) || (c.aliases ?? []).some((a) => a.startsWith(p)))
  })

  $effect(() => {
    if (sel >= suggestions.length) sel = 0
  })


  // Only the text triggers this, not whatever the handler reads.
  $effect(() => {
    const t = text
    untrack(() => oninput?.(t))
  })

  function run(name, arg = '') {
    text = ''
    onrun(name, arg)
  }

  function submit() {
    const t = text.trim()
    if (!t) return
    if (shellMode) {
      history.push(t)
      at = -1
      text = ''
      return onshell(t)
    }
    if (suggestions.length) return run(suggestions[sel].name)
    if (t.startsWith('/')) {
      const [name, ...rest] = t.slice(1).split(/\s+/)
      const c = commands.find((c) => c.name === name || (c.aliases ?? []).includes(name))
      return c ? run(c.name, rest.join(' ')) : run(name)
    }
    text = ''
    onsend(t)
  }

  const levels = ['answer', 'edit', 'commands', 'thinking']
  // Click or drag on the track: the stop nearest to the pointer.
  function slide(e) {
    const r = e.currentTarget.getBoundingClientRect()
    level = Math.max(0, Math.min(3, Math.round(((e.clientX - r.left) / r.width) * 3)))
    box?.focus()
  }

  function toggle() {
    mode = shellMode ? 'agent' : 'shell'
    box?.focus()
  }

  function onkey(e) {
    if (suggestions.length) {
      if (e.key === 'ArrowDown' || (e.key === 'Tab' && !e.shiftKey)) {
        e.preventDefault()
        if (e.key === 'Tab' && suggestions.length === 1) text = '/' + suggestions[0].name + ' '
        else sel = (sel + 1) % suggestions.length
        return
      }
      if (e.key === 'ArrowUp' || (e.key === 'Tab' && e.shiftKey)) {
        e.preventDefault()
        sel = (sel - 1 + suggestions.length) % suggestions.length
        return
      }
      if (e.key === 'Tab') return
    }
    // Tab on an empty line goes to the shell (and back, see below); so does a click on the symbol.
    if (!filtering && !shellMode && e.key === 'Tab' && !e.shiftKey && text === '') {
      e.preventDefault()
      return toggle()
    }
    if (shellMode) {
      if (e.key === 'Tab') {
        e.preventDefault()
        return text === '' ? toggle() : undefined // no completion yet
      }
      if (e.key === 'c' && e.ctrlKey && busy && !getSelection()?.toString()) {
        e.preventDefault()
        return onstop?.()
      }
      if (e.key === 'ArrowUp' && history.length && (text === '' || at >= 0) && !text.includes('\n')) {
        e.preventDefault()
        at = at < 0 ? history.length - 1 : Math.max(at - 1, 0)
        text = history[at]
        return
      }
      if (e.key === 'ArrowDown' && at >= 0) {
        e.preventDefault()
        at = at + 1 < history.length ? at + 1 : -1
        text = at < 0 ? '' : history[at]
        return
      }
    }
    if (onextra?.(e, text)) {
      e.preventDefault()
      return
    }
    if (e.key === 'Escape') text = ''
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }
</script>

<div class="wrap">
  {#if suggestions.length}
    <ul class="menu" role="listbox">
      {#each suggestions as c, i (c.name)}
        <li role="option" aria-selected={i === sel}>
          <button
            type="button"
            class:on={i === sel}
            class:off={!c.supported}
            onmousedown={(e) => e.preventDefault()}
            onclick={() => run(c.name)}
          >
            <span class="name">/{c.name}{#if c.args}<span class="args"> {c.args}</span>{/if}</span>
            <span class="desc">{c.desc}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}

  <div class="card" class:shell={shellMode}>
  <div class="context">
    <button type="button" class="chip dir" title={workdir} onclick={onworkdir}>{workdir}</button>
    {#if branchOption}
      <span class="sep">/</span>
      <Dropdown option={branchOption} prefix="⎇ " align="left" onset={(_, name) => onbranch(name)} />
      {#if git.dirty}<span class="dirty" title="Changed files">● {git.dirty}</span>{/if}
      {#if prs.length}
        <span class="sep">/</span>
        {#if prs.length === 1}
          <button type="button" class="chip pr" title={prs[0].title} onclick={() => onrun('pr', prs[0].url)}>PR #{prs[0].number}</button>
        {:else}
          <Dropdown option={prOption} prefix="PR " align="left" onset={(_, url) => onrun('pr', url)} />
        {/if}
      {/if}
    {/if}
    {#if !filtering}
    <div class="lvl" title="How much of the agent's work the chat shows">
      <span class="lname">{levels[level]}</span>
      <div class="track" role="slider" tabindex="-1" aria-label="Detail" aria-valuemin="0" aria-valuemax="3" aria-valuenow={level} aria-valuetext={levels[level]}
        onpointerdown={slide} onpointermove={(e) => e.buttons && slide(e)}>
        <span class="fill" style="width:{(level / 3) * 100}%"></span>
        {#each levels as name, i}
          <button type="button" class="stop" class:on={i <= level} class:cur={i === level} title={name} tabindex="-1" aria-label={name}
            style="left:{(i / 3) * 100}%" onmousedown={(e) => e.preventDefault()}></button>
        {/each}
      </div>
    </div>
    {/if}
  </div>

  <form class="prompt" onsubmit={(e) => { e.preventDefault(); submit() }}>
    <div class="field">
      {#if filtering}
        <span class="ps1 find" title="Type to filter the list; / runs a command" aria-hidden="true">
          <svg viewBox="0 0 16 16" width="14" height="14"><path d="M5.5 3.2 10.3 8l-4.8 4.8" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/></svg>
        </span>
      {:else}
      <button type="button" class="ps1" tabindex="-1" title={shellMode ? 'Shell: click or Tab on an empty line for the agent' : 'Agent: click or Tab on an empty line for the shell'} onmousedown={(e) => e.preventDefault()} onclick={toggle}>{shellMode ? '$' : '*'}</button>
      {/if}
      <textarea
        bind:this={box}
        bind:value={text}
        class:mono={shellMode}
        onkeydown={onkey}
        rows="2"
        placeholder={shellMode ? 'Run a command in ' + workdir : placeholder}
      ></textarea>
    </div>
    <div class="row">
      {#if filtering}
        <!-- A list or a file: the agent is not asked here, so its settings and Send are out; the search's own choice is here. -->
        <span class="note">{note || ''}</span>
        {#if searchOption}
          <span class="seg" role="radiogroup" aria-label={searchOption.label}>
            {#each searchOption.choices as c (c.id)}
              <button type="button" role="radio" aria-checked={c.id === searchOption.current} class:on={c.id === searchOption.current} title="{c.name} (Tab)" onmousedown={(e) => e.preventDefault()} onclick={() => onsearch(searchOption.key, c.id)}>{c.name}</button>
            {/each}
          </span>
        {/if}
      {:else}
        <span class="note">{note || (shellMode ? 'Enter to run · ↑ history · Tab on an empty line: back to the agent · Ctrl+C stops' : 'Enter to send · Shift+Enter for a new line')}</span>
        {#each dropdowns as o (o.key)}
          <Dropdown option={o} {onset} />
        {/each}
        {#if busy && !text.trim()}
          <button class="send" type="button" onclick={onstop}>Stop</button>
        {:else}
          <button class="send" type="submit" disabled={!text.trim()}>{shellMode ? 'Run' : busy ? 'Queue' : 'Send'}</button>
        {/if}
      {/if}
    </div>
  </form>
  </div>
</div>

<style>
  .wrap { flex: none; position: relative; width: 100%; max-width: var(--omni); margin: 0 auto; }

  .menu {
    position: absolute;
    bottom: calc(100% + 6px);
    left: 0;
    right: 0;
    max-height: 280px;
    overflow-y: auto;
    list-style: none;
    margin: 0;
    padding: 4px;
    background: var(--raised);
    border: 1px solid var(--line);
    border-radius: var(--radius);
  }
  .menu button {
    width: 100%;
    display: flex;
    align-items: baseline;
    gap: 14px;
    padding: 6px 10px;
    background: none;
    border: 0;
    border-radius: 6px;
    text-align: left;
    cursor: pointer;
  }
  .menu button.on { background: var(--accent-soft); }
  .name { color: var(--accent); font-family: var(--mono); font-size: 13px; white-space: nowrap; }
  .args { color: var(--dim); }
  .desc { color: var(--dim); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .off .name { color: var(--dim); }

  .prompt { padding: 10px 14px 10px; }

  /* The context bar is the header of the same card as the input. */
  .card {
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--radius);
    transition: border-color 0.12s;
  }
  .card:focus-within { border-color: var(--accent); }
  .card.shell:focus-within { border-color: var(--accent2); }
  .card.shell .send { background: color-mix(in srgb, var(--accent2) 18%, transparent); color: var(--accent2); }
  .card.shell .send:hover:not(:disabled) { background: var(--accent2); color: var(--bg); }

  .field { display: flex; align-items: flex-start; gap: 8px; }
  .ps1 { flex: none; width: 14px; padding: 0; background: none; border: 0; color: var(--accent); font: 600 14px/21px var(--mono); text-align: center; cursor: pointer; user-select: none; }
  .card.shell .ps1 { color: var(--accent2); }
  .ps1.find { display: grid; place-items: center; height: 21px; color: var(--accent); cursor: default; }
  textarea.mono { font-family: var(--mono); font-size: 13px; }
  .field { align-items: flex-start; }

  .context {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    padding: 5px 10px;
    border-bottom: 1px solid var(--divider);
  }
  .sep { color: var(--line); }

  /* The detail slider: four stops on a short track, the name of the current one beside it. */
  .lvl { margin-left: auto; display: flex; align-items: center; gap: 8px; }
  .lname { min-width: 4.5em; text-align: right; font-size: 11.5px; color: var(--dim); }
  .track { position: relative; width: 60px; height: 14px; margin: 0 5px; cursor: pointer; touch-action: none; }
  .track::before { content: ''; position: absolute; left: 0; right: 0; top: 6px; height: 1px; background: var(--line); }
  .fill { position: absolute; left: 0; top: 6px; height: 1px; background: var(--accent); }
  .stop { position: absolute; top: 3px; width: 7px; height: 7px; margin-left: -3.5px; padding: 0; border-radius: 50%; border: 1px solid var(--line); background: var(--panel); cursor: pointer; }
  .stop.on { border-color: var(--accent); }
  .stop.cur { background: var(--accent); transform: scale(1.3); }
  .chip {
    min-width: 0;
    max-width: 50%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    background: none;
    border: 0;
    border-radius: 5px;
    padding: 2px 8px;
    color: var(--dim);
    font: 12px var(--mono);
    cursor: pointer;
  }
  .chip:hover { background: var(--raised); color: var(--text); }
  .chip.pr { color: var(--accent); }
  .chip.pr:hover { color: var(--accent); background: var(--accent-soft); }
  .dirty { color: var(--dim); font-size: 12px; white-space: nowrap; }

  textarea {
    flex: 1;
    min-width: 0;
    width: 100%;
    resize: none;
    border: 0;
    outline: 0;
    background: none;
    color: var(--strong);
    font: inherit;
    line-height: 21px; /* the same in both modes, so that the card keeps its height */
    user-select: text;
  }
  textarea::placeholder { color: var(--dim); }

  .seg { display: flex; padding: 2px; background: var(--panel); border: 1px solid var(--line); border-radius: 7px; font-size: 12px; }
  .seg button { background: none; border: 0; border-radius: 5px; padding: 1px 11px; color: var(--dim); cursor: pointer; }
  .seg button:hover { color: var(--text); }
  .seg button.on { background: var(--accent-soft); color: var(--accent); }
  .row { display: flex; align-items: center; gap: 6px; margin-top: 6px; min-height: 28px; }
  .note { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--dim); font-size: 12px; }

  .send {
    background: var(--accent-soft);
    color: var(--accent);
    border: 0;
    border-radius: 6px;
    padding: 4px 14px;
    cursor: pointer;
  }
  .send:hover:not(:disabled) { background: var(--accent); color: var(--bg); }
  .send:disabled { opacity: 0.45; cursor: default; }
</style>
