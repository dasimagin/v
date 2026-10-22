<script>
  import Text from './Text.svelte'

  let { messages, busy, onanswer } = $props()

  let box
  let stick = true // follow the newest text unless the reader scrolled up

  function onscroll() {
    stick = box.scrollHeight - box.scrollTop - box.clientHeight < 40
  }

  $effect(() => {
    messages.length
    messages.at(-1)?.text
    busy
    if (stick && box) queueMicrotask(() => (box.scrollTop = box.scrollHeight))
  })
</script>

<section class="chat" bind:this={box} {onscroll}>
  <div class="col">
    {#each messages as m, i (i)}
      {#if m.kind === 'user'}
        <div class="user">{m.text}</div>
      {:else if m.kind === 'agent'}
        <div class="agent"><Text text={m.text} /></div>
      {:else if m.kind === 'thinking'}
        <details class="thinking"><summary>thinking</summary><div>{m.text}</div></details>
      {:else if m.kind === 'tool'}
        <div class="tool">• {m.text}</div>
      {:else if m.kind === 'ask'}
        <div class="ask" class:done={m.answer}>
          <span class="q">{m.text}</span>
          {#if m.answer}
            <span class="res" class:yes={m.answer === 'y'}>{m.answer === 'y' ? 'allowed' : 'denied'}</span>
          {:else}
            <button class="yes" onclick={() => onanswer(m, true)}>Allow</button>
            <button class="no" onclick={() => onanswer(m, false)}>Deny</button>
          {/if}
        </div>
      {:else if m.kind === 'error'}
        <div class="error">{m.text}</div>
      {:else}
        <div class="note">{m.text}</div>
      {/if}
    {/each}
    {#if busy && !messages.some((m) => m.kind === 'ask' && !m.answer)}
      <div class="working"><span class="dot"></span><span class="dot"></span><span class="dot"></span></div>
    {/if}
  </div>
</section>

<style>
  .chat { flex: 1; min-height: 0; overflow-y: auto; padding: 8px 0 14px; user-select: text; }
  .col { width: 100%; max-width: var(--col); margin: 0 auto; display: flex; flex-direction: column; gap: 12px; }

  .user {
    padding: 8px 12px;
    background: var(--panel);
    border-left: 2px solid var(--accent);
    border-radius: 0 var(--radius) var(--radius) 0;
    color: var(--strong);
    white-space: pre-wrap;
  }
  .agent { color: var(--text); line-height: 1.6; }
  .tool { color: var(--dim); font: 12px ui-monospace, 'SF Mono', Menlo, monospace; margin: -4px 0; }
  .note { color: var(--dim); font-size: 12px; }
  .error { color: var(--c-deleted, #e06c75); }

  .thinking { color: var(--dim); font-size: 12px; }
  .thinking summary { cursor: pointer; }
  .thinking div { white-space: pre-wrap; margin-top: 4px; }

  .ask {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    background: var(--accent-soft);
    border: 1px solid var(--divider);
    border-radius: var(--radius);
  }
  .ask.done { background: none; border-color: var(--line); color: var(--dim); }
  .q { flex: 1; font: 12.5px ui-monospace, 'SF Mono', Menlo, monospace; color: var(--strong); }
  .ask.done .q { color: var(--dim); }
  .ask button { border: 0; border-radius: 6px; padding: 3px 12px; cursor: pointer; font-size: 12px; }
  .ask .yes { background: var(--accent); color: var(--bg); }
  .ask .no { background: var(--raised); color: var(--text); }
  .res { font-size: 12px; color: var(--c-deleted, #e06c75); }
  .res.yes { color: var(--c-added, #98c379); }

  .working { display: flex; gap: 5px; padding: 2px 0; }
  .dot { width: 6px; height: 6px; border-radius: 50%; background: var(--dim); animation: pulse 1.2s ease-in-out infinite; }
  .dot:nth-child(2) { animation-delay: 0.2s; }
  .dot:nth-child(3) { animation-delay: 0.4s; }
  @keyframes pulse { 0%, 80%, 100% { opacity: 0.25; } 40% { opacity: 1; } }
  @media (prefers-reduced-motion: reduce) { .dot { animation: none; opacity: 0.6; } }
</style>
