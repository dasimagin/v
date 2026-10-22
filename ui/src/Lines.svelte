<script>
  // Highlighted lines: [[{t: text, c: class}]]. With numbers, a gutter shows line numbers.
  let { lines, numbers = false } = $props()
</script>

<div class="code" class:numbers>
  {#each lines as line, i (i)}
    <div class="line">
      {#if numbers}<span class="no">{i + 1}</span>{/if}<span class="text"
        >{#each line as s}<span class={s.c ? 't-' + s.c : ''}>{s.t}</span>{/each}{#if !line.length}&nbsp;{/if}</span
      >
    </div>
  {/each}
</div>

<style>
  .code { font: 12.5px/1.55 ui-monospace, 'SF Mono', Menlo, monospace; color: var(--strong); user-select: text; }
  .line { display: flex; white-space: pre; }
  .no { flex: none; width: 4.5ch; margin-right: 16px; text-align: right; color: var(--dim); opacity: 0.7; user-select: none; }
  .text { tab-size: 4; }
</style>
