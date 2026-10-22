<script>
  import Lines from './Lines.svelte'
  import Markdown from './Markdown.svelte'
  import ImageView from './ImageView.svelte'
  import RawRender from './RawRender.svelte'
  import { linesToText } from './markdown.js'
  import { view, kindOf } from './prefs.svelte.js'
  import { imageInfo } from './api.js'

  let { file, onclose } = $props()

  // Markdown can be drawn, an SVG too, a picture is drawn or described.
  const kind = $derived(kindOf(file.path))
  let picture = $state(null)
  let seq = 0
  $effect(() => {
    const path = file.path
    picture = null
    if (kind !== 'img' && kind !== 'svg') return
    const my = ++seq
    imageInfo(path).then((i) => my === seq && (picture = i))
  })
</script>

<section class="file">
  <header>
    <span class="path">{file.path}</span>
    {#if file.lang}<span class="lang">{file.lang}</span>{/if}
    <span class="spacer"></span>
    {#if kind}<RawRender {kind} />{/if}
    <button onclick={onclose} title="Close (Esc)">Close</button>
  </header>
  {#if kind === 'img'}
    <div class="body">{#if picture}<ImageView info={picture} drawn={view.img} />{/if}</div>
  {:else if kind === 'svg' && view.svg && picture}
    <div class="body"><ImageView info={picture} drawn /></div>
  {:else if file.err}
    <div class="err">{file.err}</div>
  {:else if kind === 'md' && view.md}
    <div class="body"><Markdown text={linesToText(file.lines)} /></div>
  {:else}
    <div class="body"><Lines lines={file.lines} numbers /></div>
  {/if}
</section>

<style>
  .file {
    flex: 1;
    min-height: 0;
    width: 100%;
    max-width: var(--col);
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    padding: 4px 0 14px;
  }
  header { flex: none; display: flex; align-items: center; gap: 12px; padding: 0 4px 10px; border-bottom: 1px solid var(--divider); }
  .path { color: var(--strong); font: 12.5px var(--mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .lang { color: var(--dim); font-size: 12px; }
  .spacer { flex: 1; }
  button { background: none; border: 0; border-radius: 6px; padding: 3px 10px; color: var(--dim); font-size: 12px; cursor: pointer; }
  button:hover { color: var(--text); background: var(--raised); }
  .body { flex: 1; min-height: 0; overflow: auto; padding: 10px 4px; }
  .err { padding: 16px 4px; color: var(--dim); }
</style>
