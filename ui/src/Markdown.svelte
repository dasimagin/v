<script>
  import { parse } from './markdown.js'
  import { openURL } from './api.js'

  // Markdown, drawn as elements built from the parsed tree (text nodes only: no HTML from the
  // file is ever inserted).
  let { text } = $props()
  const tree = $derived(parse(text))

  const web = (href) => /^https?:\/\//i.test(href)
  function go(e, href) {
    e.preventDefault()
    if (web(href)) openURL(href)
  }
</script>

<div class="md">{@render blocks(tree)}</div>

{#snippet inl(list)}
  {#each list as n}
    {#if n.t === 'text'}{n.text}
    {:else if n.t === 'code'}<code>{n.text}</code>
    {:else if n.t === 'strong'}<strong>{@render inl(n.kids)}</strong>
    {:else if n.t === 'em'}<em>{@render inl(n.kids)}</em>
    {:else if n.t === 'link'}<a href={web(n.href) ? n.href : undefined} class:dead={!web(n.href)} onclick={(e) => go(e, n.href)}>{@render inl(n.kids)}</a>
    {:else if n.t === 'image'}<span class="img">{n.alt || 'image'}</span>{/if}
  {/each}
{/snippet}

{#snippet blocks(list)}
  {#each list as b}
    {#if b.t === 'h'}
      <svelte:element this={'h' + b.level}>{@render inl(b.inl)}</svelte:element>
    {:else if b.t === 'p'}
      <p>{@render inl(b.inl)}</p>
    {:else if b.t === 'code'}
      <div class="block">{#if b.lang}<div class="lang">{b.lang}</div>{/if}<pre>{b.text}</pre></div>
    {:else if b.t === 'quote'}
      <blockquote>{@render blocks(b.kids)}</blockquote>
    {:else if b.t === 'hr'}
      <hr />
    {:else if b.t === 'list'}
      <svelte:element this={b.ordered ? 'ol' : 'ul'}>
        {#each b.items as it}<li>{@render blocks(it.kids)}</li>{/each}
      </svelte:element>
    {:else if b.t === 'table'}
      <div class="tbl"><table>
        <thead><tr>{#each b.head as h}<th>{@render inl(h)}</th>{/each}</tr></thead>
        <tbody>{#each b.rows as r}<tr>{#each r as c}<td>{@render inl(c)}</td>{/each}</tr>{/each}</tbody>
      </table></div>
    {/if}
  {/each}
{/snippet}

<style>
  .md { color: var(--text); line-height: 1.65; user-select: text; overflow-wrap: anywhere; }
  .md :global(h1), .md :global(h2), .md :global(h3), .md :global(h4), .md :global(h5), .md :global(h6) { color: var(--strong); line-height: 1.3; margin: 1.3em 0 0.5em; font-weight: 600; }
  .md :global(h1) { font-size: 1.7em; padding-bottom: 0.25em; border-bottom: 1px solid var(--divider); }
  .md :global(h2) { font-size: 1.35em; padding-bottom: 0.2em; border-bottom: 1px solid var(--line); }
  .md :global(h3) { font-size: 1.15em; }
  .md :global(h4), .md :global(h5), .md :global(h6) { font-size: 1em; }
  .md > :global(:first-child) { margin-top: 0; }
  p { margin: 0.6em 0; }
  code { font: 0.9em var(--mono); background: var(--raised); padding: 1px 5px; border-radius: 4px; }
  a { color: var(--accent); text-decoration: none; cursor: pointer; }
  a:hover { text-decoration: underline; }
  a.dead { cursor: default; }
  a.dead:hover { text-decoration: none; }
  .img { color: var(--dim); font-style: italic; }
  .block { margin: 0.8em 0; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); overflow: hidden; }
  .lang { padding: 3px 12px; border-bottom: 1px solid var(--line); color: var(--dim); font: 11px var(--mono); }
  pre { margin: 0; padding: 8px 12px; font: 12.5px/1.55 var(--mono); color: var(--strong); white-space: pre-wrap; overflow-wrap: anywhere; }
  blockquote { margin: 0.8em 0; padding: 0 14px; border-left: 3px solid var(--line); color: var(--dim); }
  hr { border: 0; border-top: 1px solid var(--divider); margin: 1.2em 0; }
  ul, ol { margin: 0.5em 0; padding-left: 1.6em; }
  li { margin: 0.2em 0; }
  li :global(p) { margin: 0.2em 0; }
  .tbl { overflow-x: auto; margin: 0.8em 0; }
  table { border-collapse: collapse; }
  th, td { border: 1px solid var(--line); padding: 4px 10px; text-align: left; }
  th { background: var(--panel); color: var(--strong); font-weight: 600; }
</style>
