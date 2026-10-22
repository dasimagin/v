<script>
  // A picture file, drawn (on a checkerboard, so transparency shows) or as its properties.
  let { info, drawn = true } = $props()

  const size = (n) => (n < 1024 ? n + ' B' : n < 1 << 20 ? (n / 1024).toFixed(1) + ' KB' : (n / (1 << 20)).toFixed(2) + ' MB')
  const gcd = (a, b) => (b ? gcd(b, a % b) : a)
  const ratio = $derived(info.width && info.height ? `${info.width / gcd(info.width, info.height)}:${info.height / gcd(info.width, info.height)}` : '')
</script>

{#if info.err}
  <div class="msg">{info.err}</div>
{:else if drawn}
  {#if info.data}
    <div class="stage"><img src={info.data} alt={info.name} /></div>
  {:else}
    <div class="msg">The file is {size(info.size)}, too big to show here. Its properties are under “raw”.</div>
  {/if}
{:else}
  <dl class="props">
    <dt>Name</dt><dd>{info.name}</dd>
    <dt>Type</dt><dd>{info.mime}</dd>
    <dt>Size</dt><dd>{size(info.size)}</dd>
    {#if info.width}<dt>Dimensions</dt><dd>{info.width} × {info.height} px{ratio ? ` · ${ratio}` : ''}</dd>{/if}
    <dt>Modified</dt><dd>{info.modified}</dd>
    <dt>Path</dt><dd class="path">{info.path}</dd>
  </dl>
{/if}

<style>
  .msg { color: var(--dim); padding: 8px 0; }
  .stage {
    display: grid;
    place-items: center;
    padding: 16px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: conic-gradient(var(--raised) 25%, var(--panel) 0 50%, var(--raised) 0 75%, var(--panel) 0) 0 0 / 16px 16px;
  }
  img { max-width: 100%; max-height: 60vh; object-fit: contain; image-rendering: auto; }
  .props { display: grid; grid-template-columns: max-content 1fr; gap: 6px 18px; margin: 0; }
  dt { color: var(--dim); }
  dd { margin: 0; color: var(--strong); overflow-wrap: anywhere; user-select: text; }
  .path { font: 12px var(--mono); }
</style>
