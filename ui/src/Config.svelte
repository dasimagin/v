<script>
  import { visible } from './filter.js'

  // Settings: each row is a setting with a segmented choice; a click saves it.
  let { options, query, sel, onset, onback, onselect } = $props()

  const rows = $derived(visible(options, query))

  // A choice list wider than the limit (in characters, not items) becomes a
  // carousel: the strip scrolls, arrows appear, the current value stays in view.
  const LIMIT = 36
  const long = (o) => o.choices.reduce((n, c) => n + c.name.length + 3, 0) > LIMIT

  function strip(node) {
    const edges = (v) => {
      node.dataset.start = node.scrollLeft > 1
      node.dataset.end = node.scrollLeft + node.clientWidth < node.scrollWidth - 1
      node.parentElement.classList.toggle('can-start', node.scrollLeft > 1)
      node.parentElement.classList.toggle('can-end', node.scrollLeft + node.clientWidth < node.scrollWidth - 1)
    }
    const on = node.querySelector('.on')
    if (on) node.scrollLeft = on.offsetLeft - (node.clientWidth - on.offsetWidth) / 2
    edges()
    node.addEventListener('scroll', edges, { passive: true })
    return { destroy: () => node.removeEventListener('scroll', edges) }
  }
  const step = (e, dir) => {
    const node = e.currentTarget.parentElement.querySelector('.strip')
    node.scrollBy({ left: dir * node.clientWidth * 0.7, behavior: 'smooth' })
  }
</script>

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
        <div class="seg" class:carousel={long(o)}>
          {#if long(o)}
            <button class="arrow l" tabindex="-1" aria-label="Previous" onclick={(e) => step(e, -1)}>‹</button>
          {/if}
          <div class="strip" role="radiogroup" aria-label={o.label} use:strip>
            {#each o.choices as c (c.id)}
              <button
                role="radio"
                aria-checked={c.id === o.current}
                class:on={c.id === o.current}
                onclick={() => onset(o.key, c.id)}>{c.name}</button
              >
            {/each}
          </div>
          {#if long(o)}
            <button class="arrow r" tabindex="-1" aria-label="Next" onclick={(e) => step(e, 1)}>›</button>
          {/if}
        </div>
      </li>
    {/each}
    {#if !rows.length}
      <li class="none">No settings match “{query}”</li>
    {/if}
  </ul>
</section>

<style>
  .config { flex: 1; min-height: 0; overflow-y: auto; width: 100%; max-width: var(--col); margin: 0 auto; padding: 28px 0 14px; }
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

  .seg { flex: none; display: flex; align-items: stretch; max-width: 46ch; padding: 2px; background: var(--panel); border: 1px solid var(--line); border-radius: 7px; }
  .strip { display: flex; overflow-x: hidden; scrollbar-width: none; scroll-snap-type: x proximity; }
  .strip button {
    flex: none;
    scroll-snap-align: center;
    background: none;
    border: 0;
    border-radius: 5px;
    padding: 3px 12px;
    color: var(--dim);
    cursor: pointer;
    white-space: nowrap;
  }
  .strip button:hover { color: var(--text); }
  .strip button.on { background: var(--accent-soft); color: var(--accent); }
  .carousel .strip { max-width: 30ch; }
  .arrow { background: none; border: 0; border-radius: 5px; padding: 0 7px; color: var(--dim); cursor: pointer; font-size: 16px; line-height: 1; opacity: .3; }
  .seg:global(.can-start) .arrow.l, .seg:global(.can-end) .arrow.r { opacity: 1; color: var(--accent); }
  .arrow:hover { background: var(--raised); }
</style>
