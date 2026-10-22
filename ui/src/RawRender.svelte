<script>
  import { view, setRender } from './prefs.svelte.js'

  // raw | render for a file of this kind (md, json, yaml, svg or img): the text, or the page or picture it makes.
  // For a picture, raw is its properties.
  let { kind } = $props()
  const raw = $derived(kind === 'img' ? 'The properties of the picture (Alt+R)' : 'The source (Alt+R)')
  const drawn = $derived(kind === 'json' || kind === 'yaml' ? 'A tree you can fold (Alt+R)' : 'Rendered (Alt+R)')
</script>

<span class="seg" role="group" aria-label="Raw or rendered">
  <button class:on={!view[kind]} title={raw} onclick={() => setRender(kind, false)} onmousedown={(e) => e.preventDefault()}>raw</button>
  <button class:on={view[kind]} title={drawn} onclick={() => setRender(kind, true)} onmousedown={(e) => e.preventDefault()}>render</button>
</span>

<style>
  .seg { display: flex; padding: 2px; background: var(--panel); border: 1px solid var(--line); border-radius: 7px; font: 12px 'Inter', sans-serif; }
  button { background: none; border: 0; border-radius: 5px; padding: 1px 12px; color: var(--dim); cursor: pointer; }
  button:hover { color: var(--text); }
  button.on { background: var(--accent-soft); color: var(--accent); }
</style>
