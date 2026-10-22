<script>
  import { parseData, kindOfValue, entries, preview } from './tree.js'

  // JSON or YAML as a tree: every object and list can be folded. The first two levels start open.
  let { text, kind } = $props()

  const data = $derived(parseData(text, kind))
  let folded = $state({}) // path -> true (folded) or false (open), where the reader chose
  let base = $state(2) // levels open by default; "expand all" and "fold all" change it
  let shown = $state({}) // path -> how many entries of a long list are drawn
  const LIMIT = 200

  const isOpen = (path, depth) => folded[path] ?? depth < base
  const toggle = (path, depth) => (folded[path] = isOpen(path, depth))
  function setAll(open) {
    folded = {}
    base = open ? 1000 : 1
  }
  const more = (path, n) => (shown[path] = (shown[path] ?? LIMIT) + n)
  const str = (v) => JSON.stringify(v)
  const keyText = (k) => (typeof k === 'number' ? k : String(k))
</script>

{#if !data.ok}
  <div class="bad">Not valid {kind === 'json' ? 'JSON' : 'YAML'}, so it is shown as text: {data.error}</div>
{:else}
  <div class="tree">
    <div class="tools">
      <button onclick={() => setAll(true)} onmousedown={(e) => e.preventDefault()} title="Open every node">expand all</button>
      <button onclick={() => setAll(false)} onmousedown={(e) => e.preventDefault()} title="Fold every node">fold all</button>
    </div>
    {@render node(null, data.value, '$', 0)}
  </div>
{/if}

{#snippet node(key, v, path, depth)}
  {@const k = kindOfValue(v)}
  {#if k === 'object' || k === 'array'}
    {@const open = isOpen(path, depth)}
    {@const list = entries(v)}
    <div class="row" style="padding-left:{depth * 16}px">
      <button class="car" class:open aria-label={open ? 'Fold' : 'Open'} aria-expanded={open} onclick={() => toggle(path, depth)} onmousedown={(e) => e.preventDefault()}>›</button>
      {#if key !== null}<span class="key">{keyText(key)}</span><span class="p">:</span>{/if}
      {#if open}<span class="p">{k === 'array' ? '[' : '{'}</span><span class="n">{list.length || ''}</span>
      {:else}<span class="fold" role="button" tabindex="-1" onclick={() => toggle(path, depth)} onkeydown={() => {}}>{preview(v)}</span>{/if}
    </div>
    {#if open}
      {#each list.slice(0, shown[path] ?? LIMIT) as [ck, cv] (ck)}
        {@render node(ck, cv, path + '/' + ck, depth + 1)}
      {/each}
      {#if list.length > (shown[path] ?? LIMIT)}
        <div class="row" style="padding-left:{(depth + 1) * 16 + 16}px"><button class="more" onclick={() => more(path, 500)}>… {list.length - (shown[path] ?? LIMIT)} more</button></div>
      {/if}
      <div class="row" style="padding-left:{depth * 16 + 16}px"><span class="p">{k === 'array' ? ']' : '}'}</span></div>
    {/if}
  {:else}
    <div class="row" style="padding-left:{depth * 16 + 16}px">
      {#if key !== null}<span class="key">{keyText(key)}</span><span class="p">:</span>{/if}
      <span class="v {k}">{k === 'string' ? str(v) : String(v)}</span>
    </div>
  {/if}
{/snippet}

<style>
  .bad { color: var(--dim); padding: 4px 0 10px; }
  .tree { font: 12.5px/1.6 var(--mono); user-select: text; }
  .tools { display: flex; gap: 6px; margin-bottom: 6px; font: 12px 'Inter', sans-serif; }
  .tools button { background: none; border: 1px solid var(--line); border-radius: 6px; padding: 0 9px; color: var(--dim); cursor: pointer; }
  .tools button:hover { color: var(--text); background: var(--raised); }
  .row { display: flex; align-items: baseline; gap: 6px; white-space: pre; }
  .car { flex: none; width: 14px; margin-left: -2px; padding: 0; background: none; border: 0; color: var(--dim); font: inherit; cursor: pointer; transition: transform 0.1s; }
  .car.open { transform: rotate(90deg); }
  .key { color: var(--accent); }
  .p, .n { color: var(--dim); }
  .key + .p { margin-left: -6px; }
  .n { font-size: 11px; }
  .fold { color: var(--dim); cursor: pointer; }
  .fold:hover { color: var(--text); }
  .v { white-space: pre-wrap; overflow-wrap: anywhere; }
  .v.string { color: var(--c-string, #98c379); }
  .v.number { color: var(--c-number, #d19a66); }
  .v.boolean, .v.null { color: var(--c-keyword, #c678dd); }
  .more { background: none; border: 0; padding: 0; color: var(--accent); font: inherit; cursor: pointer; }
</style>
