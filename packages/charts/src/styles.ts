/**
 * Styles for the shadow root.
 *
 * A string rather than a stylesheet asset, so a host page adds one script tag
 * and is finished. Adopted into the shadow root, which is what makes embedding
 * survivable in both directions: the host's `* { box-sizing: content-box }`
 * cannot reach in, and nothing here leaks out onto their page.
 *
 * Every colour and radius is a custom property with a fallback. Custom
 * properties are the one thing that *does* cross a shadow boundary, so they
 * are the theming API — set them on the element or anywhere above it. There is
 * no `theme` attribute with a list of our opinions.
 */
const sheet = `
$SCOPE$ {
  --cr-font: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  --cr-ink: #14140f;
  --cr-ink-secondary: #52514e;
  --cr-ink-muted: #898781;
  --cr-surface: #fff;
  --cr-line: #e4e2dd;
  /* The accent and the first series slot are one colour, and the slot is
     defined in terms of it below. A host page that themed its bar charts by
     setting --cr-accent — which was the whole API before there was a palette —
     keeps theming them. */
  --cr-accent: #2a78d6;
  --cr-good: #15803d;
  --cr-serious: #b91c1c;

  /* The categorical palette. Eight slots in a fixed order, assigned by the
     server and never cycled: a ninth series folds into the eighth rather than
     taking a generated hue, because generated hues are the ones that collide
     under colour-vision deficiency.

     The order is the safety mechanism, not decoration — it was chosen so that
     every adjacent pair clears a CVD separation floor against this surface.
     Re-ordering these is a change to what a colourblind reader can tell apart;
     re-run the palette validator if you do. */
  --cr-series-1: var(--cr-accent);
  --cr-series-2: #eb6834;
  --cr-series-3: #1baf7a;
  --cr-series-4: #eda100;
  --cr-series-5: #e87ba4;
  --cr-series-6: #008300;
  --cr-series-7: #4a3aa7;
  --cr-series-8: #e34948;

  /* The sequential ramp, for magnitude: one hue, light to dark. Never a
     rainbow — a hue scale has no order anybody agrees on, so a reader cannot
     tell which end is "more" without consulting the legend for every region. */
  --cr-ramp-1: #cde2fb;
  --cr-ramp-2: #9ec5f4;
  --cr-ramp-3: #6da7ec;
  --cr-ramp-4: #3987e5;
  --cr-ramp-5: #256abf;
  --cr-ramp-6: #104281;

  /* The ordinal ramp, for categories whose order carries meaning — funnel
     stages, treemap ranks. One hue like the sequential ramp, but with wider
     lightness gaps and a light end that still clears the surface: these are
     discrete marks a reader compares to each other, not a continuum they read
     against a legend. Five steps, because that is how many clear the gaps
     within the range the contrast floor allows. */
  --cr-step-1: #86b6ef;
  --cr-step-2: #5598e7;
  --cr-step-3: #2a78d6;
  --cr-step-4: #1c5cab;
  --cr-step-5: #104281;

  /* The diverging pair, for marks that have a sign — a waterfall's rises and
     falls. Two hues that read as opposite either side of a neutral, and
     deliberately not green/red: that pair is the one a colourblind reader
     cannot separate, and it is the one chart where the whole point is which
     side of nothing a bar is on. */
  --cr-up: #2a78d6;
  --cr-down: #e34948;
  --cr-neutral: #8c8981;
  --cr-radius: 8px;
  --cr-gap: 16px;

  display: block;
  font-family: var(--cr-font);
  color: var(--cr-ink);
  font-size: 14px;
  line-height: 1.5;
  container-type: inline-size;
}
$SCOPE$[hidden] { display: none }

* { box-sizing: border-box }

.grid {
  display: grid;
  gap: var(--cr-gap);
  grid-template-columns: repeat(auto-fit, minmax(min(220px, 100%), 1fr));
}
.wide { grid-column: 1 / -1 }

.panel {
  /* A container, so a chart inside it can ask how much room it has. The grid
     gives a panel anywhere from a third of a phone to a third of a dashboard,
     and the axis below is the part that cannot cope with both. */
  container-type: inline-size;
  background: var(--cr-surface);
  border: 1px solid var(--cr-line);
  border-radius: var(--cr-radius);
  padding: 16px;
  min-width: 0;
}
.panel h3 {
  margin: 0 0 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--cr-ink-secondary);
}

.stat { font-size: 30px; font-weight: 650; letter-spacing: -0.02em; line-height: 1.1 }
.delta { margin-top: 6px; font-size: 13px; color: var(--cr-ink-muted) }
.delta b { font-weight: 600 }
.up { color: var(--cr-good) }
.down { color: var(--cr-serious) }

table { border-collapse: collapse; width: 100%; font-variant-numeric: tabular-nums }
th, td {
  padding: 7px 8px;
  text-align: left;
  border-bottom: 1px solid var(--cr-line);
  white-space: nowrap;
}
th {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--cr-ink-muted);
}
td.r, th.r { text-align: right }
tr:last-child td { border-bottom: 0 }
.scroll { overflow-x: auto; margin: 0 -16px; padding: 0 16px }

/* Not-applicable is a first-class state, not a warning. It reads as
   information about the block, because that is what it is. */
.unaffected {
  margin-top: 10px;
  font-size: 12px;
  color: var(--cr-ink-muted);
}

.prose { margin: 0; color: var(--cr-ink-secondary); max-width: 68ch }
.msg { padding: 24px; text-align: center; color: var(--cr-ink-muted) }
.msg.err { color: var(--cr-serious) }

.bars { display: grid; gap: 2px }
.bar-row { display: grid; grid-template-columns: minmax(56px, auto) 1fr auto; gap: 10px; align-items: center }
.bar-row span { font-size: 12px; color: var(--cr-ink-secondary) }
.bar-row .v { text-align: right; font-variant-numeric: tabular-nums; color: var(--cr-ink) }
.track { height: 24px; display: flex; align-items: center }
/* 24px cap and a 4px rounded data-end, per the mark spec. A bar that grows
   past 24px stops reading as a bar and starts reading as a block of colour. */
.fill {
  height: 24px;
  background: var(--cr-accent);
  border-radius: 0 4px 4px 0;
  min-width: 2px;
}

/* -- Filter bar ---------------------------------------------------------- */

/* One row above the charts, which is where a reader looks for the thing that
   changes all of them. It wraps rather than scrolls: a control that has moved
   off the edge is one nobody knows is set. */
.bar:empty { display: none }
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 14px;
  align-items: end;
  margin-bottom: var(--cr-gap);
}
.filter { display: grid; gap: 3px; font-size: 11px; color: var(--cr-ink-muted) }
.filter .pair { display: flex; align-items: center; gap: 4px }
/* A row of toggles. With four values the list is shorter than a control that
   would hide them, and this is one tab stop per option either way. */
.chips { display: flex; flex-wrap: wrap; gap: 4px }
.chip {
  font: inherit;
  font-size: 12px;
  color: var(--cr-ink-secondary);
  background: var(--cr-surface);
  border: 1px solid var(--cr-line);
  border-radius: 999px;
  padding: 4px 10px;
  cursor: pointer;
}
.chip.on { background: var(--cr-accent); border-color: var(--cr-accent); color: #fff }
.chip:focus-visible { outline: 2px solid var(--cr-accent); outline-offset: 1px }
.filter .pair i { font-style: normal; color: var(--cr-ink-muted) }
/* Inherit, not a font stack: the host's theme reaches in through custom
   properties, and a control that ignored it would be the one element on the
   page wearing our opinion instead of theirs. */
.filter select,
.filter input {
  font: inherit;
  font-size: 13px;
  color: var(--cr-ink);
  background: var(--cr-surface);
  border: 1px solid var(--cr-line);
  border-radius: 6px;
  padding: 5px 8px;
  min-width: 0;
  max-width: 190px;
}
.filter select:focus-visible,
.filter input:focus-visible { outline: 2px solid var(--cr-accent); outline-offset: 1px }

/* -- Charts ------------------------------------------------------------- */

/* A stack's segments are separated by the panel showing through, not by a
   border. Two fills that touch read as one fill, and a border would add a
   colour that is not in the data. */
.track.stack { gap: 2px }
.track.stack .fill { border-radius: 2px }
.track.stack .fill:first-child { border-radius: 4px 2px 2px 4px }
.track.stack .fill:last-child { border-radius: 2px 4px 4px 2px }

.bar-group { display: grid; grid-template-columns: minmax(56px, auto) 1fr; gap: 10px; padding: 4px 0 }
.bar-group .bucket { font-size: 12px; color: var(--cr-ink-secondary); align-self: center }
.bar-group .series { display: grid; gap: 2px; min-width: 0 }
.bar-row.thin { grid-template-columns: 1fr auto }
.bar-row.thin .track, .bar-row.thin .fill { height: 12px }

/* The plot frame: tick labels in HTML around an SVG of marks. Keeping the
   labels out of the SVG is what stops them scaling with the panel. */
.plot {
  display: grid;
  grid-template-columns: auto 1fr;
  grid-template-areas: "ys canvas" ". xs";
  gap: 4px 8px;
  margin-top: 4px;
}
.canvas-wrap { grid-area: canvas; position: relative; min-width: 0 }
.canvas { display: block; width: 100%; height: 160px; overflow: visible }
.ys, .xs { position: relative; font-size: 11px; color: var(--cr-ink-muted); font-variant-numeric: tabular-nums }
.ys {
  grid-area: ys;
  height: 160px;
  /* A floor, because the column is sized to its content and the labels inside
     it are absolutely positioned — so the column measured zero and every tick
     was clipped to its last few characters. "60,000" rendered as "000", which
     is not a smaller label but a different number. */
  min-width: 3.4rem;
}
.ys span { position: absolute; right: 0; transform: translateY(50%); white-space: nowrap }
.xs { grid-area: xs; height: 14px }
.xs span { position: absolute; transform: translateX(-50%); white-space: nowrap }
/* Every other label, in a narrow panel.

   The server thins ticks to about six, which is what a full-width card holds;
   in a third-width one the same six run into each other and "May 2026 Jun 2026"
   renders as "May 202Bun 202". Halving them is the only thing that helps that
   a smaller font does not, and the axis is a scale rather than a list — the
   gaps still read. */
@container (max-width: 420px) {
  .xs span:nth-child(even) { display: none }
}

/* Recessive: the grid is there to be measured against, not read. */
.grid { stroke: var(--cr-line); stroke-width: 1; vector-effect: non-scaling-stroke }
.line {
  fill: none;
  stroke-width: 2;
  stroke-linejoin: round;
  stroke-linecap: round;
  /* Without this a stretched viewBox turns a 2px line into a wedge that is
     thick where the chart is wide and hairline where it is tall. */
  vector-effect: non-scaling-stroke;
}
.area { opacity: 0.16; stroke: none }
.rule { stroke: var(--cr-ink-muted); stroke-width: 1; vector-effect: non-scaling-stroke; pointer-events: none }
.rule[hidden] { display: none }
.hit { cursor: crosshair }

.dot, .pin {
  /* A 2px ring in the surface colour, so two dots that overlap still read as
     two. Without it a cluster is one blob. */
  stroke: var(--cr-surface);
  stroke-width: 1.2;
  vector-effect: non-scaling-stroke;
}
.dot:focus-visible, .pin:focus-visible, .geo path:focus-visible {
  outline: 2px solid var(--cr-accent);
  outline-offset: 2px;
}

.pie { display: block; width: 100%; max-width: 200px; margin: 4px auto }
.slices { list-style: none; margin: 8px 0 0; padding: 0; display: grid; gap: 4px }
.slices li { display: grid; grid-template-columns: auto 1fr auto; gap: 8px; align-items: center; font-size: 12px }
.slices .name { color: var(--cr-ink-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap }
.slices .v { font-variant-numeric: tabular-nums }

/* -- Maps --------------------------------------------------------------- */

.stage { position: relative; width: 100%; margin-top: 4px; overflow: hidden; border-radius: 4px }
.geo { position: relative; display: block; width: 100%; height: 100% }
.geo path { stroke: var(--cr-surface); stroke-width: 0.6; vector-effect: non-scaling-stroke }
.tiles { position: absolute; inset: 0 }
.tiles img { position: absolute; display: block }
.pin { fill: var(--cr-series-2) }
.flow { fill: none; stroke: var(--cr-series-1); stroke-opacity: 0.75; stroke-linecap: round }
.heat { pointer-events: none }
.credit { margin-top: 6px; font-size: 11px; color: var(--cr-ink-muted) }

/* -- Legend and tooltip -------------------------------------------------- */

/* Always present past one series: colour is the only thing telling series
   apart, and a colour with no name is a question. The label wears ink rather
   than the series hue — coloured 12px text fails contrast on half the palette,
   and the swatch beside it already carries the identity. */
.legend { display: flex; flex-wrap: wrap; gap: 4px 12px; margin-top: 10px; font-size: 12px }
.key { display: inline-flex; align-items: center; gap: 6px; color: var(--cr-ink-secondary) }
.swatch { width: 10px; height: 10px; border-radius: 2px; flex: none }
.legend.ramp { gap: 2px 8px; font-variant-numeric: tabular-nums }

.panel { position: relative }
.tip {
  position: absolute;
  z-index: 2;
  pointer-events: none;
  display: grid;
  gap: 1px;
  padding: 6px 8px;
  border-radius: 6px;
  background: var(--cr-ink);
  color: var(--cr-surface);
  font-size: 12px;
  line-height: 1.35;
  white-space: nowrap;
  box-shadow: 0 2px 8px rgb(0 0 0 / 0.18);
}
.tip[hidden] { display: none }
.tip span { opacity: 0.8; font-variant-numeric: tabular-nums }

/* Motion is a preference, and a report is not the place to override it. */
@media (prefers-reduced-motion: reduce) {
  * { transition: none !important; animation: none !important }
}

/* -- Combo, funnel, waterfall ------------------------------------------- */

/* Vertical bars, for the charts whose x is a sequence rather than a ranking.
   The 1px inset in the geometry plus this radius is the 2px surface gap two
   adjacent fills need to stop reading as one fill. */
.col { rx: 1 }
.thread { stroke: var(--cr-line); stroke-width: 1; stroke-dasharray: 2 2; vector-effect: non-scaling-stroke }
.rule-swatch { height: 3px; border-radius: 2px }
.axis2 { margin-top: 8px; font-size: 12px; color: var(--cr-ink-muted); display: flex; align-items: center; gap: 6px }

.funnel { display: grid; gap: 2px; margin-top: 4px }
.stage { display: grid; gap: 2px }
.stage-head { display: flex; justify-content: space-between; gap: 12px; font-size: 12px }
.stage-head .name { color: var(--cr-ink-secondary) }
.stage-head .v { font-variant-numeric: tabular-nums }
.band-row { display: flex; justify-content: center }
.band { height: 22px; border-radius: 3px; min-width: 4px }
/* The fall belongs between the two stages it describes. It keeps its height
   when empty so the first stage does not sit closer to its neighbour than the
   rest do — a funnel is read down, and an uneven rhythm reads as a gap. */
.drop {
  min-height: 14px;
  font-size: 11px;
  color: var(--cr-ink-muted);
  text-align: center;
  font-variant-numeric: tabular-nums;
}

/* -- Heatmap ------------------------------------------------------------- */

/* Rows are capped rather than square. A square cell in a full-width panel is
   300px tall, so a four-by-three grid filled the screen — the chart is read by
   comparing shades along a row, and a row taller than the eye can take in at
   once is the one thing that stops. */
.heat-grid {
  display: grid;
  grid-auto-rows: minmax(20px, 44px);
  gap: 2px;
  margin-top: 4px;
  font-size: 11px;
  align-items: stretch;
}
.col-head, .row-head { color: var(--cr-ink-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; align-self: center }
.col-head { text-align: center }
.row-head { text-align: right; padding-right: 4px }
.cell { border-radius: 2px; min-height: 20px }
/* A pair no row matched. A hairline outline rather than the ramp's lightest
   step, because "none" and "nearly none" must not look alike. */
.cell.none { background: transparent; box-shadow: inset 0 0 0 1px var(--cr-line) }

/* -- Gauge --------------------------------------------------------------- */

.gauge { display: block; width: 100%; max-width: 190px; margin: 4px auto -6px }
.gauge-value { text-align: center }
.panel .gauge + .stat { margin-top: 0 }

/* -- Treemap ------------------------------------------------------------- */

/* Capped for the reason the heatmap's rows are: at a full-width panel a 16:10
   box is most of a screen, and a treemap is read by comparing areas at a
   glance rather than by scrolling one. */
.tree {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 10;
  max-height: 300px;
  margin-top: 4px;
}
.tree-cell, .tree-frame { position: absolute; border-radius: 2px; overflow: hidden }
/* The 2px inset is the surface gap between neighbours; a border would add a
   colour that is not in the data. */
.tree-cell { outline: 2px solid var(--cr-surface); outline-offset: -2px }
.tree-frame { outline: 1px solid var(--cr-line); outline-offset: -1px; pointer-events: none }
.tree-group {
  position: absolute;
  top: 2px;
  left: 4px;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--cr-ink-muted);
}
.tree-label { position: absolute; inset: 4px auto auto 6px; display: grid; gap: 1px; line-height: 1.25 }
/* White on every step of both ramps: the ordinal ramp's light end clears 2:1
   against the surface, not against text laid over it, so the label takes the
   one colour that holds on all five steps and carries a shadow for the
   lightest. */
.tree-label b { font-size: 11px; font-weight: 600; color: #fff; text-shadow: 0 1px 2px rgb(0 0 0 / 0.45) }
.tree-label span { font-size: 10px; color: #fff; opacity: 0.85; text-shadow: 0 1px 2px rgb(0 0 0 / 0.45) }

@media (prefers-color-scheme: dark) {
  $SCOPE$ {
    --cr-ink: #f5f4f1;
    --cr-ink-secondary: #b9b6ae;
    --cr-ink-muted: #8c8981;
    --cr-surface: #17171a;
    --cr-line: #2c2c31;
    --cr-good: #4ade80;
    --cr-serious: #f87171;
    /* Re-stepped for the dark surface, like every other hue here. */
    --cr-accent: #3987e5;

    /* The same eight hues, re-stepped for a dark surface — not the light
       values with a filter over them. An automatic flip puts half the palette
       below 3:1 against this background, which is the difference between a
       series being a colour and a series being a smudge. */
    --cr-series-1: var(--cr-accent);
    --cr-series-2: #d95926;
    --cr-series-3: #199e70;
    --cr-series-4: #c98500;
    --cr-series-5: #d55181;
    --cr-series-6: #008300;
    --cr-series-7: #9085e9;
    --cr-series-8: #e66767;

    /* Reversed, because "near zero recedes toward the surface" means dark
       here. A ramp that stayed light-to-dark would paint the emptiest regions
       brightest and the busiest ones invisible. */
    --cr-ramp-1: #104281;
    --cr-ramp-2: #1c5cab;
    --cr-ramp-3: #2a78d6;
    --cr-ramp-4: #5598e7;
    --cr-ramp-5: #86b6ef;
    --cr-ramp-6: #b7d3f6;

    /* Re-stepped for the dark surface, and reversed for the same reason the
       sequential ramp is: the step nearest the surface has to stay visible
       against it. */
    --cr-step-1: #184f95;
    --cr-step-2: #256abf;
    --cr-step-3: #3987e5;
    --cr-step-4: #6da7ec;
    --cr-step-5: #9ec5f4;

    --cr-up: #3987e5;
    --cr-down: #e66767;
  }
}
`

/**
 * The stylesheet, scoped to wherever it is being used.
 *
 * `:host` only exists inside a shadow root, and the portal draws these charts
 * in an ordinary document. Substituting the selector keeps one stylesheet for
 * both rather than a copy that drifts — and the custom properties come with it,
 * so a host page themes the embed and the portal themes itself through exactly
 * the same names.
 */
export function css(scope = ':host'): string {
  return sheet.replaceAll('$SCOPE$', scope)
}
