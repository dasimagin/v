<script>
  import { visible } from './filter.js'

  // Settings: each row is a setting with a segmented choice; a click saves it.
  let { options, query, sel, onset, onback, onselect } = $props()

  const rows = $derived(visible(options, query))

  // A choice list wider than the limit (in characters, not items) becomes a
  // dropdown that opens downward and scrolls; short ones stay segmented.
  const LIMIT = 36
  const long = (o) => o.choices.reduce((n, c) => n + c.name.length + 3, 0) > LIMIT

  let openKey = $state('')
  let pop = $state()
  $effect(() => {
    if (openKey && pop) pop.querySelector('.cur')?.scrollIntoView({ block: 'nearest' })
  })
  const nameOf = (o) => o.choices.find((c) => c.id === o.current)?.name ?? o.current
  function pick(o, id) {
    openKey = ''
    onset(o.key, id)
  }
  function onwindow(e) {
    if (!openKey) return
    if (e.type === 'keydown' ? e.key === 'Escape' : !e.target.closest?.('.dd')) {
      openKey = ''
      if (e.type === 'keydown') e.stopPropagation()
    }
  }
</script>

<svelte:window onmousedown={onwindow} onkeydown={onwindow} />

<section class="config">
  <header>
    <button class="back" onclick={onback} title="Back (Esc)">← Back</button>
    <h1>Settings</h1>
  </header>

  <ul>
    {#each rows as o, i (o.key)}
      <li class:sel={i === sel} onmouseenter={() => onselect(i)}>
        <div class="text">
          <div class="label">{o.label}</div>
          <div class="desc">{o.desc}</div>
        </div>
        {#if long(o)}
          <div class="dd">
            <button class="sel-btn" class:open={openKey === o.key} aria-haspopup="listbox" aria-expanded={openKey === o.key} onclick={() => (openKey = openKey === o.key ? '' : o.key)}>
              {nameOf(o)} <span class="caret">▾</span>
            </button>
            {#if openKey === o.key}
              <ul class="pop" role="listbox" bind:this={pop}>
                {#each o.choices as c (c.id)}
                  <li><button role="option" aria-selected={c.id === o.current} class:cur={c.id === o.current} onclick={() => pick(o, c.id)}>
                    <span class="tick">{c.id === o.current ? '✓' : ''}</span>{c.name}
                  </button></li>
                {/each}
              </ul>
            {/if}
          </div>
        {:else}
          <div class="seg" role="radiogroup" aria-label={o.label}>
            {#each o.choices as c (c.id)}
              <button role="radio" aria-checked={c.id === o.current} class:on={c.id === o.current} onclick={() => onset(o.key, c.id)}>{c.name}</button>
            {/each}
          </div>
        {/if}
      </li>
    {/each}
    {#if !rows.length}
      <li class="none">No settings match “{query}”</li>
    {/if}
  </ul>
</section>

<style>
  .config { flex: 1; min-height: 0; overflow-y: auto; width: 100%; max-width: var(--omni); margin: 0 auto; padding: 28px 0 14px; }
  .config > header, .config > ul { padding-inline: 0; }

  header { display: flex; align-items: center; gap: 14px; margin-bottom: 18px; padding-left: 16px; }
  h1 { margin: 0; font-size: 18px; font-weight: 600; color: var(--strong); }
  .back {
    background: none;
    border: 0;
    border-radius: 6px;
    padding: 4px 8px;
    color: var(--dim);
    cursor: pointer;
  }
  .back:hover { color: var(--text); background: var(--raised); }

  ul { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--divider); }
  li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 24px;
    padding: 14px 20px 14px 24px;
    border-bottom: 1px solid var(--line);
  }
  li.sel { background: var(--accent-soft); }
  li.none { color: var(--dim); justify-content: center; }
  .label { color: var(--strong); }
  .desc { color: var(--dim); font-size: 12px; margin-top: 2px; }

  .seg { flex: none; display: flex; padding: 2px; background: var(--panel); border: 1px solid var(--line); border-radius: 7px; }
  .seg button {
    background: none;
    border: 0;
    border-radius: 5px;
    padding: 3px 12px;
    color: var(--dim);
    cursor: pointer;
    white-space: nowrap;
  }
  .seg button:hover { color: var(--text); }
  .seg button.on { background: var(--accent-soft); color: var(--accent); }

  .dd { position: relative; flex: none; }
  .sel-btn {
    min-width: 170px;
    display: flex;
    justify-content: space-between;
    gap: 12px;
    padding: 5px 10px;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 7px;
    color: var(--text);
    cursor: pointer;
  }
  .sel-btn:hover, .sel-btn.open { border-color: var(--accent); }
  .caret { color: var(--dim); font-size: 12px; align-self: center; }
  .pop {
    position: absolute;
    left: 50%;
    transform: translateX(-50%);
    top: calc(100% + 4px);
    z-index: 10;
    min-width: 100%;
    max-height: 240px;
    overflow-y: auto;
    list-style: none;
    margin: 0;
    padding: 3px;
    background: var(--raised);
    border: 1px solid var(--line);
    border-radius: var(--radius);
    box-shadow: 0 8px 24px rgb(0 0 0 / 0.3);
  }
  .pop button {
    width: 100%;
    display: flex;
    gap: 8px;
    align-items: center;
    background: none;
    border: 0;
    border-radius: 5px;
    padding: 2px 10px 2px 4px;
    font-size: 12.5px;
    line-height: 1.4;
    text-align: left;
    white-space: nowrap;
    color: var(--text);
    cursor: pointer;
  }
  .pop button:hover { background: var(--accent-soft); }
  .pop button.cur { color: var(--accent); }
  .tick { width: 10px; font-size: 11px; color: var(--accent); }
</style>
