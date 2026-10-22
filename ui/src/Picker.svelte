<script>
  import Lines from './Lines.svelte'
  import CommitView from './CommitView.svelte'
  import BranchView from './BranchView.svelte'
  import Markdown from './Markdown.svelte'
  import ImageView from './ImageView.svelte'
  import Tree from './Tree.svelte'
  import RawRender from './RawRender.svelte'
  import { linesToText } from './markdown.js'
  import { view, kindOf } from './prefs.svelte.js'
  import { imageInfo } from './api.js'

  // A list on the left and a preview of the selected row on the right.
  let { kind, pick, sel, preview, empty, onselect, onchoose, onjump } = $props()

  let list
  $effect(() => {
    list?.querySelector('.on')?.scrollIntoView({ block: 'nearest' })
  })

  // The selected file, when it can be shown in two ways.
  const chosen = $derived(kind === 'open' ? pick?.items?.[sel] : null)
  const fileKind = $derived(chosen && !chosen.dir ? kindOf(chosen.value) : '')
  let picture = $state(null) // the properties and data of the selected picture or SVG
  let pictureSeq = 0
  $effect(() => {
    const path = chosen?.value
    const k = fileKind
    picture = null
    if (!path || (k !== 'img' && k !== 'svg')) return
    const my = ++pictureSeq
    imageInfo(path).then((i) => my === pictureSeq && (picture = i))
  })
</script>

<section class="picker">
  {#if pick?.crumbs?.length}
    <div class="bar">
      <nav class="crumbs" aria-label="Folders">
        {#each pick.crumbs as c, i (i)}
          {#if i > 0}<span class="sep">›</span>{/if}
          {#if i === pick.crumbs.length - 1}
            <span class="here">{c.name}</span>
          {:else}
            <button class="crumb" onclick={() => onjump(c.query)} onmousedown={(e) => e.preventDefault()}>{c.name}</button>
          {/if}
        {/each}
      </nav>
      <div class="switch">{#if fileKind}<RawRender kind={fileKind} />{/if}</div>
    </div>
  {/if}
  <div class="cols">
  <div class="list" bind:this={list}>
    {#if pick?.dir && !pick?.crumbs?.length}<div class="dir">{pick.dir}</div>{/if}
    {#if pick?.err}<div class="msg">{pick.err}</div>{/if}
    {#each pick?.items ?? [] as it, i (it.value)}
      {#if it.below && !pick.items[i - 1]?.below}
        <div class="split" role="separator"><span>below</span></div>
      {/if}
      <button class="row" class:on={i === sel} class:isdir={it.dir} onclick={() => onselect(i)} ondblclick={() => onchoose(i)}>
        {#if kind === 'branch'}
          <span class="top">
            <span class="label">{it.label}</span>
            {#if it.tag === 'current'}<span class="badge cur">current</span>{:else if it.tag === 'remote'}<span class="badge">remote</span>{/if}
          </span>
          {#if it.sub}<span class="meta">{it.sub}{it.when ? ' · ' + it.when : ''}</span>{/if}
        {:else if kind === 'log'}
          <span class="logtext">
            <span class="logtop"><span class="label">{it.label}</span>{#each (it.refs ?? []).slice(0, 3) as r}<span class="ref {r.kind}">{#if r.kind === 'tag'}<svg viewBox="0 0 16 16" width="10" height="10" aria-hidden="true"><path fill="currentColor" d="M1.5 2.5h6l6 6-5 5-6-6v-5Zm2.2 1.2a1 1 0 1 0 0 2 1 1 0 0 0 0-2Z"/></svg>{/if}{r.name}</span>{/each}{#if (it.refs ?? []).length > 3}<span class="ref more">+{it.refs.length - 3}</span>{/if}</span>
            {#if it.meta}<span class="meta"><code>{it.meta.split(' · ')[0]}</code> {it.meta.split(' · ').slice(1).join(' · ')}</span>{/if}
          </span>
        {:else}
          <span class="label">{it.label}</span>
          {#if it.meta}<span class="meta">{it.meta}</span>{/if}
        {/if}
      </button>
    {/each}
    {#if pick && !pick.items.length && !pick.err}<div class="msg">{empty}</div>{/if}
  </div>
  <div class="pane">
    {#if kind === 'log' && preview?.kind === 'commit'}
      <CommitView d={preview} />
    {:else if kind === 'branch' && preview?.kind === 'branch'}
      <BranchView d={preview} />
    {:else if fileKind === 'md' && view.md && Array.isArray(preview)}
      <Markdown text={linesToText(preview)} />
    {:else if (fileKind === 'json' || fileKind === 'yaml') && view[fileKind] && Array.isArray(preview)}
      <Tree text={linesToText(preview)} kind={fileKind} />
    {:else if fileKind === 'img' && picture}
      <ImageView info={picture} drawn={view.img} />
    {:else if fileKind === 'svg' && view.svg && picture}
      <ImageView info={picture} drawn />
    {:else if fileKind === 'img'}
      <div class="msg">…</div>
    {:else if Array.isArray(preview)}
      <Lines lines={preview} />
    {/if}
  </div>
  </div>
</section>

<style>
  .picker {
    flex: 1;
    min-height: 0;
    width: 100%;
    max-width: var(--col);
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    padding: 4px 0 14px;
  }
  /* The bar over the list: the folders above this one, each a jump back; the view switch on the right. */
  .bar { flex: none; display: flex; align-items: center; justify-content: space-between; gap: 12px; height: 36px; padding: 0 4px 8px; margin-bottom: 8px; border-bottom: 1px solid var(--divider); box-sizing: border-box; }
  /* The switch has its place whether it is shown or not, so that nothing moves when it appears. */
  .switch { flex: none; width: 124px; height: 28px; display: flex; align-items: center; justify-content: flex-end; }
  .crumbs { display: flex; align-items: center; flex-wrap: nowrap; overflow: hidden; white-space: nowrap; gap: 2px; min-width: 0; font: 12.5px var(--mono); }
  .crumb { background: none; border: 0; border-radius: 5px; padding: 1px 6px; color: var(--dim); font: inherit; cursor: pointer; }
  .crumb:hover { color: var(--accent); background: var(--accent-soft); }
  .here { padding: 1px 6px; color: var(--strong); font-weight: 600; }
  .sep { color: var(--line); }
  .cols {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(220px, 2fr) 3fr;
    gap: 16px;
  }
  .list, .pane { min-height: 0; overflow: auto; }
  .pane { border-left: 1px solid var(--divider); padding: 0 0 0 16px; }

  /* Matches in this folder above, matches in the folders below under the line. */
  .split { display: flex; align-items: center; gap: 8px; margin: 8px 10px 6px; color: var(--dim); font-size: 11px; text-transform: uppercase; letter-spacing: 0.08em; }
  .split::before, .split::after { content: ''; flex: 1; height: 1px; background: var(--divider); }
  .split::before { flex: 0 0 8px; }
  .dir { padding: 2px 10px 8px; color: var(--dim); font: 12px var(--mono); }
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
  .badge { flex: none; margin-left: auto; padding: 0 7px; border-radius: 999px; border: 1px solid var(--line); color: var(--dim); font-size: 11px; }
  .badge.cur { border-color: transparent; background: var(--accent-soft); color: var(--accent); }
  .logtext { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
  .logtop { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .logtop .label { flex: 0 1 auto; min-width: 0; }
  .ref { flex: none; display: inline-flex; align-items: center; gap: 3px; padding: 0 6px; border-radius: 999px; font: 11px/1.5 var(--mono); border: 1px solid var(--line); color: var(--dim); }
  .ref.head { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 50%, transparent); background: var(--accent-soft); }
  .ref.branch { color: var(--c-added, #98e379); border-color: color-mix(in srgb, var(--c-added, #98e379) 40%, transparent); }
  .ref.remote { color: var(--accent2); border-color: color-mix(in srgb, var(--accent2) 40%, transparent); }
  .ref.tag { color: #e5c07b; border-color: color-mix(in srgb, #e5c07b 45%, transparent); }
  .meta code { color: var(--accent); font: 11.5px var(--mono); }
  .meta { color: var(--dim); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
