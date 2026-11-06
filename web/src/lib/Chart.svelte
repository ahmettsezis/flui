<script>
  import uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';

  let { title, ts, series, height = 170, percent = false } = $props();
  let el = $state(), chart = null, observer = null;

  function styleVar(name) {
    return getComputedStyle(el).getPropertyValue(name).trim();
  }

  function build() {
    const fg = styleVar('--muted') || '#64748b';
    chart = new uPlot({
      title,
      width: el.clientWidth,
      height,
      scales: { x: { time: true }, y: { auto: true, range: percent ? [0, 100] : undefined } },
      axes: [
        { stroke: fg, grid: { stroke: 'rgba(128,128,128,0.15)' }, ticks: { stroke: 'rgba(128,128,128,0.3)' } },
        { stroke: fg, grid: { stroke: 'rgba(128,128,128,0.15)' }, size: 46, ticks: { stroke: 'rgba(128,128,128,0.3)' } },
      ],
      series: [{}, ...series],
      legend: { show: series.length > 1 },
    }, [ts, ...series.map(s => s.data)], el);
  }

  $effect(() => {
    build();
    observer = new ResizeObserver(() => chart.setSize({ width: el.clientWidth, height }));
    observer.observe(el);
    return () => { observer?.disconnect(); chart?.destroy(); };
  });
</script>

<div class="chart" bind:this={el}></div>

<style>
  .chart { width: 100%; }
  .chart :global(.u-title) { font-size: 0.85rem; font-weight: 600; color: var(--fg); }
  .chart :global(.u-legend) { font-size: 0.75rem; color: var(--muted); }
</style>
