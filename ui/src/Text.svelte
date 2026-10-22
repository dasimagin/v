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
    <pre class="block"><code>{p.text}</code></pre>
  {:else}
    <span class="para">{#each inline(p.text) as s}{#if s.c}<code class="inl">{s.t}</code>{:else}{s.t}{/if}{/each}</span>
  {/if}
{/each}

<style>
  .para { white-space: pre-wrap; }
  .inl { font: 0.92em ui-monospace, 'SF Mono', Menlo, monospace; background: var(--raised); padding: 1px 5px; border-radius: 4px; }
  .block {
    margin: 8px 0;
    padding: 10px 12px;
    overflow-x: auto;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--radius);
    font: 12.5px/1.55 ui-monospace, 'SF Mono', Menlo, monospace;
    color: var(--strong);
    user-select: text;
  }
</style>
