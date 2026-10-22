<script>
  // Agent text: paragraphs as written, ``` fences as code blocks, `inline code`.
  let { text } = $props()

  const parts = $derived.by(() => {
    const out = []
    const re = /```(\w*)\n([\s\S]*?)(```|$)/g
    let at = 0
    for (let m; (m = re.exec(text)); ) {
      if (m.index > at) out.push({ code: false, text: text.slice(at, m.index).replace(/^\n+|\n+$/g, '') })
      out.push({ code: true, lang: m[1], text: m[2].replace(/\n$/, '') })
      at = re.lastIndex
    }
    if (at < text.length) out.push({ code: false, text: text.slice(at).replace(/^\n+|\n+$/g, '') })
    return out.filter((p) => p.code || p.text)
  })

  const inline = (t) => t.split(/(`[^`\n]+`)/g).map((s) => (s.length > 2 && s.startsWith('`') && s.endsWith('`') ? { c: true, t: s.slice(1, -1) } : { c: false, t: s }))
</script>

{#each parts as p}
  {#if p.code}
    <div class="block">
      <div class="lang">{p.lang || 'code'}</div>
      <pre><code>{p.text}</code></pre>
    </div>
  {:else}
    <span class="para">{#each inline(p.text) as s}{#if s.c}<code class="inl">{s.t}</code>{:else}{s.t}{/if}{/each}</span>
  {/if}
{/each}

<style>
  .para { white-space: pre-wrap; }
  .inl { font: 0.92em var(--mono); background: var(--raised); padding: 1px 5px; border-radius: 4px; }
  /* A code block is a panel of its own: the language on top, the code under it. */
  .block { margin: 8px 0; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); overflow: hidden; }
  .lang { padding: 3px 12px; border-bottom: 1px solid var(--line); color: var(--dim); font: 11px var(--mono); text-transform: lowercase; }
  .block pre { margin: 0; padding: 8px 12px; font: 12.5px/1.55 var(--mono); color: var(--strong); white-space: pre-wrap; overflow-wrap: anywhere; user-select: text; }
</style>
