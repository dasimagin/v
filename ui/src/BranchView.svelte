<script>
  import { copyText } from './copy.js'

  // The right side of /branch: the branch's name (a click copies it), how it stands against the current
  // branch, and its latest commits as the list of /log has them: the subject with its tags and branches, and
  // under it the hash, the author and the time.
  let { d } = $props()
  let copied = $state(false)
  async function copy() {
    await copyText(d.ref)
    copied = true
    setTimeout(() => (copied = false), 1400)
  }
</script>

{#if d.err}
  <div class="err">{d.err}</div>
{:else}
  <header>
    <button class="name" class:ok={copied} title={copied ? 'Copied' : `Copy ${d.ref}`} onclick={copy}>{copied ? 'copied' : d.ref}</button>
    {#if d.isCurrent}
      <span class="chip cur">you are on this branch</span>
    {:else}
      <span class="chip" class:zero={!d.ahead}><b>{d.ahead}</b> ahead of {d.current}</span>
      <span class="chip" class:zero={!d.behind}><b>{d.behind}</b> behind</span>
      <span class="chip"><span class="add">+{d.add}</span> <span class="del">−{d.del}</span> in {d.files.length} file{d.files.length === 1 ? '' : 's'}</span>
    {/if}
  </header>

  <ul class="commits">
    {#each d.commits as c, i (c.short + i)}
      <li>
        <span class="top">
          <span class="subject">{c.subject}</span>
          {#each (c.refs ?? []).slice(0, 3) as r}<span class="ref {r.kind}">{r.name}</span>{/each}{#if (c.refs ?? []).length > 3}<span class="ref more">+{c.refs.length - 3}</span>{/if}
        </span>
        <span class="meta"><code>{c.short}</code> {c.author} · {c.rel}</span>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .err { color: var(--dim); }
  header { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; padding: 2px 10px 10px; }
  .name { background: none; border: 0; padding: 0; margin-right: 4px; color: var(--strong); font: 600 16px 'Inter', sans-serif; cursor: pointer; text-align: left; word-break: break-all; }
  .name:hover { color: var(--accent); }
  .name.ok { color: var(--c-added, #98e379); }
  .chip { padding: 1px 9px; border: 1px solid var(--line); border-radius: 999px; background: var(--panel); color: var(--text); font-size: 12px; }
  .chip b { color: var(--accent); }
  .chip.zero, .chip.zero b { color: var(--dim); }
  .chip.cur { background: var(--accent-soft); color: var(--accent); border-color: transparent; }
  .add { color: var(--c-added); }
  .del { color: var(--c-deleted); }

  .commits { list-style: none; margin: 0; padding: 0; }
  li { display: flex; flex-direction: column; gap: 1px; padding: 5px 10px; border-radius: 6px; }
  li:hover { background: var(--raised); }
  .top { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .subject { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--strong); }
  .meta { color: var(--dim); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta code { color: var(--accent); font: 11.5px var(--mono); }
  .ref { flex: none; display: inline-flex; align-items: center; padding: 0 6px; border-radius: 999px; font: 11px/1.5 var(--mono); border: 1px solid var(--line); color: var(--dim); }
  .ref.head { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 50%, transparent); background: var(--accent-soft); }
  .ref.branch { color: var(--c-added, #98e379); border-color: color-mix(in srgb, var(--c-added, #98e379) 40%, transparent); }
  .ref.remote { color: var(--accent2); border-color: color-mix(in srgb, var(--accent2) 40%, transparent); }
  .ref.tag { color: #e5c07b; border-color: color-mix(in srgb, #e5c07b 45%, transparent); }
</style>
