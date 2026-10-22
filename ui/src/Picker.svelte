<script>
  import Lines from './Lines.svelte'
  import CommitView from './CommitView.svelte'
  import BranchView from './BranchView.svelte'

  // A list on the left and a preview of the selected row on the right.
  let { kind, pick, sel, preview, empty, onselect, onchoose } = $props()

  let list
  $effect(() => {
    list?.querySelector('.on')?.scrollIntoView({ block: 'nearest' })
  })
</script>

<section class="picker">
  <div class="list" bind:this={list}>
    {#if pick?.dir}<div class="dir">{pick.dir}</div>{/if}
    {#if pick?.err}<div class="msg">{pick.err}</div>{/if}
    {#each pick?.items ?? [] as it, i (it.value)}
      <button class="row" class:on={i === sel} class:isdir={it.dir} onclick={() => onselect(i)} ondblclick={() => onchoose(i)}>
        {#if kind === 'switch'}
          <span class="top">
            <svg class="bi" viewBox="0 0 16 16" width="13" height="13" aria-hidden="true"><path fill="currentColor" d="M5 3.25a1.75 1.75 0 1 1-2.5 1.58v6.34a1.75 1.75 0 1 1 1.5 0V8.5c0-.97.78-1.75 1.75-1.75h3.5a.25.25 0 0 0 .25-.25V4.83a1.75 1.75 0 1 1 1.5 0V6.5A1.75 1.75 0 0 1 9.5 8.25H6a.25.25 0 0 0-.25.25v.67A1.75 1.75 0 0 1 5 3.25Z"/></svg>
            <span class="label">{it.label}</span>
            {#if it.tag === 'current'}<span class="badge cur">current</span>{:else if it.tag === 'remote'}<span class="badge">remote</span>{/if}
          </span>
          {#if it.sub}<span class="meta">{it.sub}{it.when ? ' · ' + it.when : ''}</span>{/if}
        {:else if kind === 'show'}
          <span class="label">{it.label}</span>
          {#if it.meta}<span class="meta"><code>{it.meta.split(' · ')[0]}</code> {it.meta.split(' · ').slice(1).join(' · ')}</span>{/if}
        {:else}
          <span class="label">{it.label}</span>
          {#if it.meta}<span class="meta">{it.meta}</span>{/if}
        {/if}
      </button>
    {/each}
    {#if pick && !pick.items.length && !pick.err}<div class="msg">{empty}</div>{/if}
  </div>
  <div class="pane">
    {#if kind === 'show' && preview?.kind === 'commit'}
      <CommitView d={preview} />
    {:else if kind === 'switch' && preview?.kind === 'branch'}
      <BranchView d={preview} />
    {:else if Array.isArray(preview)}
      <Lines lines={preview} />
    {/if}
  </div>
</section>

<style>
  .picker {
    flex: 1;
    min-height: 0;
    width: 100%;
    max-width: var(--col);
    margin: 0 auto;
    display: grid;
    grid-template-columns: minmax(220px, 2fr) 3fr;
    gap: 16px;
    padding: 4px 0 14px;
  }
  .list, .pane { min-height: 0; overflow: auto; }
  .pane { border-left: 1px solid var(--divider); padding: 0 0 0 16px; }

  .dir { padding: 2px 10px 8px; color: var(--dim); font: 12px ui-monospace, 'SF Mono', Menlo, monospace; }
  .msg { padding: 8px 10px; color: var(--dim); }
  .row {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding: 5px 10px;
    background: none;
    border: 0;
    border-radius: 6px;
    text-align: left;
    cursor: pointer;
  }
  .row:hover { background: var(--raised); }
  .row.on { background: var(--accent-soft); }
  .label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--strong); }
  .isdir .label { color: var(--accent); }
  .top { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .bi { flex: none; color: var(--dim); }
  .on .bi { color: var(--accent); }
  .badge { flex: none; margin-left: auto; padding: 0 7px; border-radius: 999px; border: 1px solid var(--line); color: var(--dim); font-size: 11px; }
  .badge.cur { border-color: transparent; background: var(--accent-soft); color: var(--accent); }
  .meta code { color: var(--accent); font: 11.5px ui-monospace, 'SF Mono', Menlo, monospace; }
  .meta { color: var(--dim); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
