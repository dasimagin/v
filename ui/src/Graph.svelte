<script>
  // One row of the history graph: lanes side by side, the commit's dot, and the lines that run
  // past it, join it from above or go on to its parents.
  let { row, lanes = 1, height = 50 } = $props()

  const COLORS = ['#61afef', '#98e379', '#e5c07b', '#e06cdd', '#56d6e0', '#e06c75', '#c678dd', '#d19a66']
  const STEP = 13
  const x = (l) => 8 + Math.min(l, 7) * STEP
  const color = (l) => COLORS[Math.min(l, 7) % COLORS.length]
  const mid = $derived(height / 2)
  const width = $derived(8 + Math.min(lanes, 8) * STEP)
</script>

<svg class="graph" {width} {height} viewBox="0 0 {width} {height}" aria-hidden="true">
  {#each row.pass as l}
    <line x1={x(l)} y1="0" x2={x(l)} y2={height} stroke={color(l)} stroke-width="1.6" />
  {/each}
  {#each row.in as l}
    <path d="M{x(l)} 0 C{x(l)} {mid * 0.7} {x(row.node)} {mid * 0.3} {x(row.node)} {mid}" fill="none" stroke={color(l)} stroke-width="1.6" />
  {/each}
  {#each row.out as l}
    <path d="M{x(row.node)} {mid} C{x(row.node)} {mid * 1.3} {x(l)} {mid * 1.7} {x(l)} {height}" fill="none" stroke={color(l)} stroke-width="1.6" />
  {/each}
  <circle cx={x(row.node)} cy={mid} r="4" fill={color(row.node)} stroke="var(--bg)" stroke-width="1.5" />
</svg>

<style>
  .graph { flex: none; display: block; margin-right: 8px; }
</style>
