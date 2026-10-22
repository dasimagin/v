<script>
  import Lines from './Lines.svelte'
  import Markdown from './Markdown.svelte'
  import ImageView from './ImageView.svelte'
  import RawRender from './RawRender.svelte'
  import Editor from './Editor.svelte'
  import { linesToText } from './markdown.js'
  import { view, kindOf } from './prefs.svelte.js'
  import { imageInfo } from './api.js'

  // A file: as text you can edit (raw), or drawn (Markdown, SVG), or, for a picture, drawn or described.
  let { file, dirty = $bindable(false), query = '', mode = $bindable('regexp'), onclose, onescape } = $props()
  let editor
  // Next (1) or previous (-1) match of the omnibox's text in the file.
  export const find = (dir) => editor?.find(dir)

  const kind = $derived(kindOf(file.path))
  const rendered = $derived((kind === 'md' && view.md) || (kind === 'svg' && view.svg))
  let text = $state('') // what the editor holds right now, saved or not
  let picture = $state(null)
  let seq = 0
  $effect(() => {
    const path = file.abs ?? file.path
    picture = null
    if (kind !== 'img' && kind !== 'svg') return
    const my = ++seq
    imageInfo(path).then((i) => my === seq && (picture = i))
  })

  // The SVG as it is being edited, for the drawn view.
  const svgInfo = $derived(
    picture && file.edit && dirty ? { ...picture, data: 'data:image/svg+xml;base64,' + btoa(unescape(encodeURIComponent(text))) } : picture,
  )
  const source = $derived(file.edit ? text : linesToText(file.lines))

</script>

<section class="file">
  <header>
    <span class="path">{file.path}</span>
    {#if file.lang}<span class="lang">{file.lang}</span>{/if}
    <span class="spacer"></span>
    {#if kind}<RawRender {kind} />{/if}
  </header>
  {#if kind === 'img'}
    <div class="body">{#if picture}<ImageView info={picture} drawn={view.img} />{/if}</div>
  {:else if file.err}
    <div class="err">{file.err}</div>
  {:else}
    {#if rendered}
      <div class="body">
        {#if kind === 'md'}<Markdown text={source} />{:else if svgInfo}<ImageView info={svgInfo} drawn />{/if}
      </div>
    {/if}
    <div class="edit" hidden={rendered}>
      {#if file.edit}
        <Editor bind:this={editor} {file} {query} bind:mode {onescape} bind:text bind:dirty />
      {:else}
        <div class="body"><Lines lines={file.lines} numbers /></div>
      {/if}
    </div>
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
  .body { flex: 1; min-height: 0; overflow: auto; padding: 10px 4px; }
  .edit { flex: 1; min-height: 0; display: flex; flex-direction: column; }
  .edit[hidden] { display: none; }
  .err { padding: 16px 4px; color: var(--dim); }
</style>
