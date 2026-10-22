<script>
  // A commit as a card: who and when, the message, the files with their +/−
  // bars, then each file's patch folded under its name.
  let { d } = $props()

  const total = $derived(d.files.reduce((n, f) => ({ a: n.a + f.add, r: n.r + f.del }), { a: 0, r: 0 }))
  const initial = $derived((d.author || '?').trim()[0]?.toUpperCase() ?? '?')
  const bar = (f) => {
    const t = f.add + f.del || 1
    return { a: Math.round((f.add / t) * 100), r: Math.round((f.del / t) * 100) }
  }
  const dir = (p) => (p.includes('/') ? p.slice(0, p.lastIndexOf('/') + 1) : '')
  const base = (p) => p.slice(p.lastIndexOf('/') + 1)
  let open = $state({})
</script>

{#if d.err}
  <div class="err">{d.err}</div>
{:else}
  <article>
    <header>
      <span class="avatar">{initial}</span>
      <div class="who">
        <div class="name">{d.author}</div>
        <div class="when">{d.rel} · {d.date}</div>
      </div>
      <code class="hash" title={d.hash}>{d.short}</code>
    </header>

    <h2>{d.subject}</h2>
    {#if d.body}<p class="body">{d.body}</p>{/if}

    <div class="sum">
      {d.files.length + d.moreFiles} files changed
      <span class="add">+{total.a}</span> <span class="del">−{total.r}</span>
    </div>

    {#each d.files as f, i (f.path)}
      <section class="file">
        <button class="fh" onclick={() => (open[i] = !open[i])} aria-expanded={!!open[i]}>
          <span class="car" class:open={open[i]}>›</span>
          <span class="path"><span class="dirp">{dir(f.path)}</span>{base(f.path)}</span>
          <span class="bar" aria-hidden="true"><i class="a" style="width:{bar(f).a}%"></i><i class="r" style="width:{bar(f).r}%"></i></span>
          <span class="nums"><span class="add">+{f.add}</span> <span class="del">−{f.del}</span></span>
        </button>
        {#if open[i]}
          <div class="patch">
            {#if f.binary}<div class="note">Binary file</div>{/if}
            {#each f.rows as r}
              <div class="dl {r.k}"><span class="no">{r.old || ''}</span><span class="no">{r.new || ''}</span><span class="t">{r.k === 'add' ? '+' : r.k === 'del' ? '−' : ' '}{r.t}</span></div>
            {/each}
            {#if f.more}<div class="note">… {f.more} more lines</div>{/if}
          </div>
        {/if}
      </section>
    {/each}
    {#if d.moreFiles}<div class="note">… {d.moreFiles} more files</div>{/if}
  </article>
{/if}

<style>
  .err { color: var(--dim); }
  header { display: flex; align-items: center; gap: 10px; }
  .avatar {
    width: 30px; height: 30px; border-radius: 50%; display: grid; place-items: center;
    background: linear-gradient(135deg, var(--accent), var(--accent2)); color: var(--bg); font-weight: 700;
  }
  .who { flex: 1; min-width: 0; }
  .name { color: var(--strong); font-weight: 600; }
  .when { color: var(--dim); font-size: 12px; }
  .hash { background: var(--accent-soft); color: var(--accent); padding: 2px 8px; border-radius: 6px; font: 12px ui-monospace, 'SF Mono', Menlo, monospace; }
  h2 { margin: 14px 0 6px; font-size: 16px; line-height: 1.35; color: var(--strong); }
  .body { margin: 0 0 4px; white-space: pre-wrap; color: var(--text); line-height: 1.5; }
  .sum { margin: 16px 0 8px; padding-top: 12px; border-top: 1px solid var(--divider); color: var(--dim); font-size: 12px; }
  .add { color: var(--c-added, #3fb950); }
  .del { color: var(--c-deleted, #f85149); }

  .file { border: 1px solid var(--line); border-radius: 8px; margin-bottom: 6px; overflow: hidden; background: var(--panel); }
  .fh { width: 100%; display: flex; align-items: center; gap: 8px; padding: 7px 10px; background: none; border: 0; text-align: left; cursor: pointer; }
  .fh:hover { background: var(--raised); }
  .car { color: var(--dim); transition: transform .12s; width: 10px; }
  .car.open { transform: rotate(90deg); }
  .path { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--strong); font: 12.5px ui-monospace, 'SF Mono', Menlo, monospace; }
  .dirp { color: var(--dim); }
  .bar { flex: none; width: 48px; height: 5px; border-radius: 3px; background: var(--line); display: flex; overflow: hidden; }
  .bar .a { background: var(--c-added, #3fb950); }
  .bar .r { background: var(--c-deleted, #f85149); }
  .nums { flex: none; font-size: 12px; min-width: 64px; text-align: right; }

  .patch { border-top: 1px solid var(--line); overflow-x: auto; font: 12px/1.5 ui-monospace, 'SF Mono', Menlo, monospace; }
  .dl { display: flex; white-space: pre; min-width: max-content; }
  .dl .no { flex: none; width: 38px; text-align: right; padding-right: 8px; color: var(--dim); opacity: .6; user-select: none; }
  .dl .t { padding-right: 12px; }
  .dl.add { background: color-mix(in srgb, var(--c-added, #3fb950) 14%, transparent); }
  .dl.del { background: color-mix(in srgb, var(--c-deleted, #f85149) 14%, transparent); }
  .dl.hunk { color: var(--accent); background: var(--accent-soft); }
  .dl.hunk .t { padding-left: 12px; }
  .note { padding: 6px 12px; color: var(--dim); font-size: 12px; }
</style>
