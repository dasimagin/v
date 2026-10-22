<script>
  // A small pill showing the current value of a setting; a click opens the
  // choices upward. A pick calls onset(key, id).
  let { option, onset, prefix = '', align = 'right' } = $props()

  let open = $state(false)
  let host

  const label = $derived(option.choices.find((c) => c.id === option.current)?.name ?? option.current)

  function choose(id) {
    open = false
    onset(option.key, id)
  }

  function onwindow(e) {
    if (!open) return
    if (e.type === 'keydown' ? e.key === 'Escape' : !host?.contains(e.target)) open = false
  }
</script>

<svelte:window onmousedown={onwindow} onkeydown={onwindow} />

<div class="host" bind:this={host}>
  <button type="button" class="pill" class:on={open} title={option.label} onclick={() => (open = !open)}>
    {prefix}{label} <span class="caret">▾</span>
  </button>
  {#if open}
    <ul class="pop" class:left={align === 'left'} role="listbox">
      {#each option.choices as c (c.id)}
        <li>
          <button type="button" class:cur={c.id === option.current} onclick={() => choose(c.id)}>{c.name}</button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .host { position: relative; }
  .pill {
    background: none;
    border: 0;
    border-radius: 5px;
    padding: 2px 8px;
    color: var(--text);
    font-size: 12px;
    cursor: pointer;
  }
  .pill:hover, .pill.on { background: var(--raised); }
  .caret { color: var(--dim); font-size: 10px; }

  .pop {
    position: absolute;
    right: 0;
    bottom: calc(100% + 6px);
    min-width: 130px;
    max-height: 260px;
    overflow-y: auto;
    z-index: 5;
    list-style: none;
    margin: 0;
    padding: 4px;
    background: var(--raised);
    border: 1px solid var(--line);
    border-radius: var(--radius);
  }
  .pop.left { left: 0; right: auto; }
  .pop button {
    width: 100%;
    background: none;
    border: 0;
    border-radius: 5px;
    padding: 5px 10px;
    text-align: left;
    color: var(--text);
    cursor: pointer;
  }
  .pop button:hover { background: var(--accent-soft); }
  .pop button.cur { color: var(--accent); }
</style>
