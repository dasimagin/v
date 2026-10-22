<script>
  import Text from './Text.svelte'

  // A pull request: Conversation and the changed files on the left; the
  // description and talk, or the diff of a file with its review threads, on the right.
  let { pr, rows, sel, view, onselect } = $props()
</script>

<section class="pr">
  <div class="list">
    <div class="head">
      <div class="title">{pr.title}</div>
      <div class="meta">
        #{pr.number} by @{pr.author} · {pr.state}{pr.draft ? ', draft' : ''} · {pr.base} ← {pr.head}
      </div>
    </div>
    {#each rows as r, i (r.path)}
      <button class="row" class:on={i === sel} onclick={() => onselect(i)}>
        <span class="label" class:conv={r.path === ''}>{r.label}</span>
        <span class="stat">
          {#if r.path === ''}
            {pr.files.length} files
          {:else}
            <span class="add">+{r.adds}</span> <span class="del">−{r.dels}</span>{#if r.threads} · <span class="th">{r.threads} 💬</span>{/if}
          {/if}
        </span>
      </button>
    {/each}
    {#if !rows.length}<div class="msg">Nothing matches</div>{/if}
  </div>

  <div class="pane">
    {#if rows[sel]?.path === ''}
      <div class="conv">
        {#if pr.labels.length}
          <div class="labels">{#each pr.labels as l}<span class="label-chip">{l}</span>{/each}</div>
        {/if}
        <div class="body"><Text text={view.body || '(no description)'} /></div>
        {#each view.reviews as r}
          <div class="box" class:approved={r.state === 'approved'} class:changes={r.state === 'changes requested'}>
            <div class="by"><b>{r.author}</b> <span class="badge">{r.state}</span> <span class="when">{r.when}</span></div>
            {#if r.body}<div class="text"><Text text={r.body} /></div>{/if}
          </div>
        {/each}
        {#each view.comments as c}
          <div class="box">
            <div class="by"><b>{c.author}</b> <span class="when">{c.when}</span></div>
            <div class="text"><Text text={c.body} /></div>
          </div>
        {/each}
      </div>
    {:else}
      <div class="diff">
        {#each view.lines as l}
          <div class="dl" class:add={l.k === 'add'} class:del={l.k === 'del'} class:hunk={l.k === 'hunk'} class:head={l.k === 'head'} class:meta={l.k === 'meta'} class:ctx={l.k === 'ctx'}><span class="no">{l.no || ''}</span><span class="t">{l.t || ' '}</span></div>
          {#each l.threads ?? [] as t}
            {@render thread(t)}
          {/each}
        {/each}
        {#if view.outdated.length}
          <div class="out">Outdated</div>
          {#each view.outdated as t}
            {@render thread(t)}
          {/each}
        {/if}
      </div>
    {/if}
  </div>
</section>

{#snippet thread(t)}
  <div class="thread" class:resolved={t.resolved}>
    {#if t.resolved || t.outdated}
      <div class="flags">{t.resolved ? 'resolved' : ''}{t.resolved && t.outdated ? ' · ' : ''}{t.outdated ? 'outdated' : ''}</div>
    {/if}
    {#each t.comments as c}
      <div class="tc">
        <div class="by"><b>{c.author}</b> <span class="when">{c.when}</span></div>
        <div class="text"><Text text={c.body} /></div>
      </div>
    {/each}
  </div>
{/snippet}

<style>
  .pr {
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
  .pane { border-left: 1px solid var(--divider); padding-left: 16px; }

  .head { padding: 2px 10px 10px; }
  .title { color: var(--strong); font-weight: 600; }
  .meta { color: var(--dim); font-size: 12px; margin-top: 2px; }

  .row {
    width: 100%;
    display: flex;
    justify-content: space-between;
    gap: 10px;
    padding: 5px 10px;
    background: none;
    border: 0;
    border-radius: 6px;
    text-align: left;
    cursor: pointer;
  }
  .row:hover { background: var(--raised); }
  .row.on { background: var(--accent-soft); }
  .label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--strong); direction: rtl; text-align: left; }
  .label.conv { color: var(--accent); direction: ltr; }
  .stat { flex: none; color: var(--dim); font-size: 12px; }
  .add { color: var(--c-added); }
  .del { color: var(--c-deleted); }
  .th { color: var(--accent); }
  .msg { padding: 8px 10px; color: var(--dim); }

  .conv { display: flex; flex-direction: column; gap: 12px; user-select: text; }
  .labels { display: flex; gap: 6px; }
  .label-chip { background: var(--accent-soft); color: var(--accent); border-radius: 10px; padding: 1px 8px; font-size: 12px; }
  .body { color: var(--text); line-height: 1.6; }
  .box { border: 1px solid var(--line); border-radius: var(--radius); padding: 8px 12px; }
  .box.approved { border-color: var(--c-added); }
  .box.changes { border-color: var(--c-deleted); }
  .by { font-size: 12px; color: var(--text); }
  .when { color: var(--dim); }
  .badge { color: var(--dim); }
  .text { margin-top: 4px; color: var(--text); line-height: 1.55; }

  .diff { font: 12.5px/1.55 ui-monospace, 'SF Mono', Menlo, monospace; user-select: text; }
  .dl { display: flex; white-space: pre; }
  .no { flex: none; width: 4.5ch; margin-right: 12px; text-align: right; color: var(--dim); opacity: 0.7; user-select: none; }
  .dl.add { background: color-mix(in srgb, var(--c-added) 14%, transparent); }
  .dl.del { background: color-mix(in srgb, var(--c-deleted) 14%, transparent); }
  .dl.add .t { color: var(--c-added); }
  .dl.del .t { color: var(--c-deleted); }
  .dl.hunk .t { color: var(--c-func); }
  .dl.head .t { color: var(--strong); font-weight: 600; }
  .dl.meta .t { color: var(--dim); }
  .dl.ctx .t { color: var(--strong); }

  .thread {
    margin: 4px 0 8px calc(4.5ch + 12px);
    padding: 6px 12px;
    font: 13px/1.5 -apple-system, BlinkMacSystemFont, 'Inter', sans-serif;
    background: var(--panel);
    border: 1px solid var(--line);
    border-left: 2px solid var(--accent);
    border-radius: 0 var(--radius) var(--radius) 0;
    white-space: normal;
  }
  .thread.resolved { opacity: 0.6; border-left-color: var(--dim); }
  .flags { font-size: 11px; color: var(--dim); text-transform: uppercase; letter-spacing: 0.06em; }
  .tc + .tc { margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--line); }
  .out { margin: 12px 0 4px; color: var(--dim); font-size: 12px; }
</style>
