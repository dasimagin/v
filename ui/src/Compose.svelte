<script>
  import { onMount } from 'svelte'

  // A box to write a comment, a reply or a summary in. Ctrl/Cmd+Enter sends, Esc cancels.
  let { placeholder = 'Write a comment…', label = 'Comment', initial = '', busy = false, rows = 3, onsubmit, oncancel } = $props()

  // svelte-ignore state_referenced_locally
  let text = $state(initial)
  const mod = /Mac/.test(navigator.platform) ? '⌘' : 'Ctrl+'
  let box

  onMount(() => box?.focus())

  function key(e) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      send()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      oncancel?.()
    }
  }

  function send() {
    if (text.trim() && !busy) onsubmit(text)
  }
</script>

<div class="compose">
  <textarea bind:this={box} bind:value={text} onkeydown={key} {placeholder} {rows} disabled={busy}></textarea>
  <div class="bar">
    <span class="hint">{mod}Enter sends · Esc cancels</span>
    {#if oncancel}<button type="button" class="ghost" onclick={oncancel} disabled={busy}>Cancel</button>{/if}
    <button type="button" class="go" onclick={send} disabled={busy || !text.trim()}>{busy ? 'Sending…' : label}</button>
  </div>
</div>

<style>
  .compose { border: 1px solid var(--accent); border-radius: 8px; background: var(--bg); padding: 6px 8px 6px; }
  textarea { width: 100%; resize: vertical; min-height: 3.2em; border: 0; outline: 0; background: none; color: var(--strong); font: 13px/1.5 'Inter', sans-serif; user-select: text; }
  textarea::placeholder { color: var(--dim); }
  .bar { display: flex; align-items: center; gap: 8px; margin-top: 4px; }
  .hint { flex: 1; color: var(--dim); font-size: 11px; }
  button { border: 0; border-radius: 6px; padding: 3px 12px; cursor: pointer; font-size: 12px; }
  .ghost { background: none; color: var(--dim); }
  .ghost:hover { background: var(--raised); color: var(--text); }
  .go { background: var(--accent); color: var(--bg); }
  .go:disabled { opacity: 0.45; cursor: default; }
</style>
