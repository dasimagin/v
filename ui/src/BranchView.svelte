<script>
  // A branch as a card: how it stands against the current branch, what it
  // changes, and a timeline of its latest commits.
  let { d } = $props()
  const dir = (p) => (p.includes('/') ? p.slice(0, p.lastIndexOf('/') + 1) : '')
  const base = (p) => p.slice(p.lastIndexOf('/') + 1)
</script>

{#if d.err}
  <div class="err">{d.err}</div>
{:else}
  <article>
    <h2>
      <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M5 3.25a1.75 1.75 0 1 1-2.5 1.58v6.34a1.75 1.75 0 1 1 1.5 0V8.5c0-.97.78-1.75 1.75-1.75h3.5a.25.25 0 0 0 .25-.25V4.83a1.75 1.75 0 1 1 1.5 0V6.5A1.75 1.75 0 0 1 9.5 8.25H6a.25.25 0 0 0-.25.25v.67A1.75 1.75 0 0 1 5 3.25Z"/></svg>
      {d.ref}
    </h2>

    {#if d.isCurrent}
      <div class="state cur">You are on this branch</div>
    {:else}
      <div class="chips">
        <span class="chip" class:zero={!d.ahead}><b>{d.ahead}</b> ahead of {d.current}</span>
        <span class="chip" class:zero={!d.behind}><b>{d.behind}</b> behind</span>
        <span class="chip"><span class="add">+{d.add}</span> <span class="del">−{d.del}</span> in {d.files.length} files</span>
      </div>
      {#if d.files.length}
        <div class="h">Changes against {d.current}</div>
        <ul class="files">
          {#each d.files as f (f.path)}
            <li><span class="path"><span class="dirp">{dir(f.path)}</span>{base(f.path)}</span><span class="nums"><span class="add">+{f.add}</span> <span class="del">−{f.del}</span></span></li>
          {/each}
        </ul>
      {/if}
    {/if}

    <div class="h">Latest commits</div>
    <ol class="tl">
      {#each d.commits as c, i (c.short + i)}
        <li>
          <span class="dot"></span>
          <div class="subj">{c.subject}</div>
          <div class="meta"><code>{c.short}</code> · {c.author} · {c.rel}</div>
        </li>
      {/each}
    </ol>
  </article>
{/if}

<style>
  .err { color: var(--dim); }
  h2 { display: flex; align-items: center; gap: 8px; margin: 0 0 12px; font-size: 16px; color: var(--strong); word-break: break-all; }
  h2 svg { color: var(--accent); flex: none; }
  .state { padding: 8px 12px; border-radius: 8px; background: var(--accent-soft); color: var(--accent); margin-bottom: 8px; }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .chip { padding: 3px 10px; border: 1px solid var(--line); border-radius: 999px; background: var(--panel); color: var(--text); font-size: 12px; }
  .chip b { color: var(--accent); }
  .chip.zero { color: var(--dim); }
  .chip.zero b { color: var(--dim); }
  .add { color: var(--c-added, #3fb950); }
  .del { color: var(--c-deleted, #f85149); }
  .h { margin: 18px 0 8px; padding-top: 12px; border-top: 1px solid var(--divider); color: var(--dim); font-size: 12px; text-transform: uppercase; letter-spacing: .06em; }

  .files { list-style: none; margin: 0; padding: 0; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); max-height: 220px; overflow: auto; }
  .files li { display: flex; justify-content: space-between; gap: 12px; padding: 5px 10px; font: 12.5px ui-monospace, 'SF Mono', Menlo, monospace; }
  .files li + li { border-top: 1px solid var(--line); }
  .path { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--strong); }
  .dirp { color: var(--dim); }
  .nums { flex: none; }

  .tl { list-style: none; margin: 0; padding: 0 0 0 4px; }
  .tl li { position: relative; padding: 0 0 12px 20px; border-left: 2px solid var(--line); margin-left: 4px; }
  .tl li:last-child { border-left-color: transparent; }
  .dot { position: absolute; left: -6px; top: 4px; width: 10px; height: 10px; border-radius: 50%; background: var(--bg); border: 2px solid var(--accent); }
  .tl li:first-child .dot { background: var(--accent); }
  .subj { color: var(--strong); line-height: 1.35; }
  .meta { color: var(--dim); font-size: 12px; margin-top: 2px; }
  .meta code { color: var(--accent); font: 12px ui-monospace, 'SF Mono', Menlo, monospace; }
</style>
