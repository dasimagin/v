<script>
  // The home logo: a smooth V with a bright gradient that flows through it in a
  // seamless loop (the shift is exactly one period, so the end meets the start),
  // and no outline: a soft halo (dark themes) or shadow (light themes) lifts it off the page.
  let { animate = true } = $props()

  let flow // the gradient element
  const V = 'M8 6 H52 L96 112 L140 6 H184 L118 168 H74 Z'

  // The gradient flows by moving its vector along itself by exactly one period
  // (.7, .7), so the loop is seamless. It is driven from here and not by SMIL,
  // because WebKit does not animate gradientTransform.
  // An effect, not onMount: the logo is first drawn before the settings arrive,
  // when animate is still false, and has to start when it turns true.
  $effect(() => {
    if (!animate || matchMedia('(prefers-reduced-motion: reduce)').matches) return
    let raf
    const frame = (t) => {
      const o = ((t % 6000) / 6000) * 0.7
      flow.setAttribute('x1', o)
      flow.setAttribute('y1', o)
      flow.setAttribute('x2', o + 0.7)
      flow.setAttribute('y2', o + 0.7)
      raf = requestAnimationFrame(frame)
    }
    raf = requestAnimationFrame(frame)
    return () => cancelAnimationFrame(raf)
  })
</script>

<div class="logo" role="img" aria-label="v">
  <svg viewBox="-14 -18 220 212" xmlns="http://www.w3.org/2000/svg">
    <defs>
      <!-- One period is the vector (.7,.7); the animation shifts by exactly that. -->
      <linearGradient id="flow" bind:this={flow} x1="0" y1="0" x2=".7" y2=".7" spreadMethod="repeat">
        <stop offset="0" stop-color="#61afef" />
        <stop offset=".25" stop-color="#56d6e0" />
        <stop offset=".5" stop-color="#98e379" />
        <stop offset=".72" stop-color="#e5c07b" />
        <stop offset=".88" stop-color="#e06cdd" />
        <stop offset="1" stop-color="#61afef" />
      </linearGradient>
      <linearGradient id="shine" x1="0" y1="0" x2="0" y2="1">
        <stop class="top" offset="0" stop-color="#fff" stop-opacity=".5" />
        <stop offset=".55" stop-color="#fff" stop-opacity="0" />
      </linearGradient>
    </defs>

    <path d={V} fill="url(#flow)" />
    <path d={V} fill="url(#shine)" />
  </svg>
</div>

<style>
  .logo { width: 190px; height: 190px; flex: none; }
  svg { width: 100%; height: 100%; overflow: visible; }

  /* No outline: a soft white halo on dark themes, a soft dark shadow on light ones. */
  svg { filter: drop-shadow(0 0 8px rgb(255 255 255 / 0.35)); }
  :global([data-mode='light']) svg { filter: drop-shadow(0 2px 4px rgb(31 36 48 / 0.2)); }
  :global([data-mode='light']) .top { stop-opacity: 0.2; }
  @media (max-height: 520px) { .logo { width: 84px; height: 84px; } }
</style>
