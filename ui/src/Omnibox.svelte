<script>
  import { untrack } from 'svelte'
  import Dropdown from './Dropdown.svelte'

  // The input at the bottom: plain text goes to the agent, "/" opens command suggestions.
  let { commands, options, onset, note, busy = false, onstop, workdir, git, branches, onbranch, onworkdir, placeholder, onrun, onsend, oninput, onkey: onextra } = $props()

  let text = $state('')
  let sel = $state(0)
  let box
  const branchOption = $derived(
    git?.branch ? { key: 'branch', label: 'Branch', current: git.branch, choices: branches.map((b) => ({ id: b, name: b })) } : null,
  )
  const dropdowns = $derived(['model', 'effort'].map((k) => options.find((o) => o.key === k)).filter(Boolean))

  export function setText(t) {
    text = t
    box?.focus()
  }

  export function focus() {
    box?.focus()
  }

  // Suggestions show while a command name is being typed (no space yet).
  const suggestions = $derived.by(() => {
    if (!text.startsWith('/') || /\s/.test(text)) return []
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
    if (suggestions.length) return run(suggestions[sel].name)
    if (t.startsWith('/')) {
      const [name, ...rest] = t.slice(1).split(/\s+/)
      const c = commands.find((c) => c.name === name || (c.aliases ?? []).includes(name))
      return c ? run(c.name, rest.join(' ')) : run(name)
    }
    text = ''
    onsend(t)
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

  <div class="card">
  <div class="context">
    <button type="button" class="chip dir" title={workdir} onclick={onworkdir}>{workdir}</button>
    {#if branchOption}
      <span class="sep">/</span>
      <Dropdown option={branchOption} prefix="⎇ " align="left" onset={(_, name) => onbranch(name)} />
      {#if git.dirty}<span class="dirty" title="Changed files">● {git.dirty}</span>{/if}
    {/if}
  </div>

  <form class="prompt" onsubmit={(e) => { e.preventDefault(); submit() }}>
    <textarea
      bind:this={box}
      bind:value={text}
      onkeydown={onkey}
      rows="2"
      {placeholder}
    ></textarea>
    <div class="row">
      <span class="note">{note || 'Enter to send · Shift+Enter for a new line'}</span>
      {#each dropdowns as o (o.key)}
        <Dropdown option={o} {onset} />
      {/each}
      {#if busy && !text.trim()}
        <button class="send" type="button" onclick={onstop}>Stop</button>
      {:else}
        <button class="send" type="submit" disabled={!text.trim()}>{busy ? 'Queue' : 'Send'}</button>
      {/if}
    </div>
  </form>
  </div>
</div>

<style>
  .wrap { flex: none; position: relative; width: 100%; max-width: var(--col); margin: 0 auto; }

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
  .name { color: var(--accent); font-family: ui-monospace, 'SF Mono', Menlo, monospace; font-size: 13px; white-space: nowrap; }
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

  .context {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    padding: 5px 10px;
    border-bottom: 1px solid var(--divider);
  }
  .sep { color: var(--line); }
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
    font: 12px ui-monospace, 'SF Mono', Menlo, monospace;
    cursor: pointer;
  }
  .chip:hover { background: var(--raised); color: var(--text); }
  .dirty { color: var(--dim); font-size: 12px; white-space: nowrap; }

  textarea {
    width: 100%;
    resize: none;
    border: 0;
    outline: 0;
    background: none;
    color: var(--strong);
    font: inherit;
    user-select: text;
  }
  textarea::placeholder { color: var(--dim); }

  .row { display: flex; align-items: center; gap: 6px; margin-top: 6px; }
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
