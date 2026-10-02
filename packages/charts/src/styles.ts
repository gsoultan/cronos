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
/* The theme, as custom properties: light, and the same names re-stepped for a
   dark surface. Held once each and placed three times below — as the default,
   under the reader's own dark setting, and under a theme the page names — so
   the lists cannot drift apart. */
const LIGHT = `
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
  /* The ink a number on each shade takes: dark on the light half, white on
     the dark half. Beside the shades they go with, so a page that swaps one
     swaps both. */
  --cr-on-ramp-1: #14140f;
  --cr-on-ramp-2: #14140f;
  --cr-on-ramp-3: #14140f;
  --cr-on-ramp-4: #fff;
  --cr-on-ramp-5: #fff;
  --cr-on-ramp-6: #fff;

  /* The diverging ramp, for a measure with a middle — a change, a margin: the
     sequential ramp's blue below it and the palette's orange above, darkest
     furthest from it and palest either side. Blue against orange holds apart
     under every common colour-vision deficiency, which red against green
     does not. A map reads it through the sequential names; see map.ts. */
  --cr-div-1: #104281;
  --cr-div-2: #3987e5;
  --cr-div-3: #9ec5f4;
  --cr-div-4: #f7c6ab;
  --cr-div-5: #eb7f4c;
  --cr-div-6: #a33f12;

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

`

const DARK = `
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
    --cr-on-ramp-1: #fff;
    --cr-on-ramp-2: #fff;
    --cr-on-ramp-3: #fff;
    --cr-on-ramp-4: #0b1526;
    --cr-on-ramp-5: #0b1526;
    --cr-on-ramp-6: #0b1526;
    /* Reversed for the same reason: the shades either side of the midpoint
       recede toward the surface, and the extremes are the bright ones. */
    --cr-div-1: #b7d3f6;
    --cr-div-2: #5598e7;
    --cr-div-3: #1c5cab;
    --cr-div-4: #8a3510;
    --cr-div-5: #d95926;
    --cr-div-6: #f5b28f;

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
`

const sheet = `
$SCOPE$ {
${LIGHT}  display: block;
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
  margin: 0 0 10px;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: -0.005em;
  color: var(--cr-ink);
}

/* The width a stat's figure is fitted to. The second size is the one that
   fits; the first is for a browser without container units. */
.stat-fit { container-type: inline-size }
.stat { margin: 4px 0 0; font-size: 32px; font-size: clamp(14px, calc(100cqi / (var(--n, 8) * 0.64)), 32px); font-weight: 650; letter-spacing: -0.025em; line-height: 1.1; font-variant-numeric: tabular-nums }
.delta { margin: 8px 0 0; font-size: 13px; color: var(--cr-ink-muted) }
.delta b { font-weight: 600 }
.up { color: var(--cr-good) }
.down { color: var(--cr-serious) }
/* A change, in a pill tinted by whether it is good news — the engine's call,
   not the arrow's. */
.pill { display: inline-block; padding: 1px 8px; border-radius: 999px; font-size: 12px; font-variant-numeric: tabular-nums }
.pill.up { background: color-mix(in srgb, var(--cr-good) 13%, transparent) }
.pill.down { background: color-mix(in srgb, var(--cr-serious) 13%, transparent) }

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

/* Every chart is an SVG drawn at its panel's width in pixels (see frame.ts),
   so nothing here scales a mark: a stroke is as wide as it says, text is the
   size it says, and a circle stays one. */
.chart-host { position: relative; min-width: 0; margin-top: 2px }
.canvas { display: block; overflow: visible }
.canvas text, .pie text, .gauge text { font-family: inherit }

/* Text in a chart, by role. Muted for the scale, secondary for names, ink for
   the numbers a reader came for. */
.tick { font-size: 11px; fill: var(--cr-ink-muted); font-variant-numeric: tabular-nums }
.name { font-size: 12px; fill: var(--cr-ink-secondary) }
.value { font-size: 12px; font-weight: 550; fill: var(--cr-ink); font-variant-numeric: tabular-nums }
.drop { font-size: 11px; fill: var(--cr-ink-muted); font-variant-numeric: tabular-nums }

/* Recessive: the grid is there to be measured against, not read. The line at
   nothing is firmer, because every bar stands on it. */
.gridline { stroke: var(--cr-line); stroke-width: 1; shape-rendering: crispEdges }
.gridline.base, .gridline.zero { stroke: color-mix(in srgb, var(--cr-ink-muted) 55%, transparent) }

.line { fill: none; stroke-width: 2.25; stroke-linejoin: round; stroke-linecap: round }
.area { stroke: none }
.area.stacked { opacity: 0.82 }
/* A ring in the surface colour, so a dot on a line and two dots that touch
   still read as separate marks. */
.point { stroke: var(--cr-surface); stroke-width: 2 }
.rule { stroke: var(--cr-ink-muted); stroke-width: 1; stroke-dasharray: 3 3; pointer-events: none }
.focus { pointer-events: none }
.hit { fill: transparent; stroke: none; cursor: crosshair }
.thread { stroke: var(--cr-ink-muted); stroke-width: 1; stroke-dasharray: 3 3; opacity: 0.7 }

/* A bullet's track: its target's fractions in three greys, darkest furthest
   from it, and the target a firm mark across the bar. */
.shade { stroke: none }
.shade.s0 { fill: color-mix(in srgb, var(--cr-ink-muted) 34%, var(--cr-surface)) }
.shade.s1 { fill: color-mix(in srgb, var(--cr-ink-muted) 20%, var(--cr-surface)) }
.shade.s2 { fill: color-mix(in srgb, var(--cr-ink-muted) 9%, var(--cr-surface)) }
.target { stroke: var(--cr-ink); stroke-width: 2.5; stroke-linecap: round }

/* A radar's shapes are washes with their edges drawn, so one behind another
   still shows through. */
.radar { fill-opacity: 0.14; stroke-width: 2; stroke-linejoin: round }
/* A radar's ring is a closed path, and a closed path fills black unless told
   not to. Not .ring: that is a map's radius, washed in its category's hue.
   Its scale sits inside the web, where a shape's edge can cross it, so each
   figure is haloed in the surface. */
.web-ring { fill: none }

/* A box plot: the box washed in its hue with its edge drawn, the median the
   firmest line on it, the whiskers hairlines in the scale's ink. */
.box-body { fill-opacity: 0.16; stroke-width: 1.5 }
.median { stroke-width: 2.5; stroke-linecap: round }
.whisker { stroke: var(--cr-ink-muted); stroke-width: 1.25 }
.marks:hover .box:not(:hover) { opacity: 0.5 }

/* A sankey's bands in their source's colour, softened, and the ones through
   a node pointed at lit while the rest fall back. A target is drawn in the
   scale's ink: its colour would claim it is one of the sources. */
.flow-band { fill-opacity: 0.32; transition: fill-opacity 0.15s }
.flow-band:hover, .flow-band.on { fill-opacity: 0.6 }
.tracing .flow-band:not(.on) { fill-opacity: 0.08 }
.flow-node { fill: var(--cr-ink-muted) }

/* A stat's line: its shape under the figure, washed, each period's point
   there to be pointed at and seen only then, the latest one always. */
.spark { margin-top: 8px; max-width: 100% }
/* A chart wider than its panel at the smallest it can be drawn — a year of
   days — scrolls sideways rather than shrinking its squares to specks. */
.chart-host.scrolls { overflow-x: auto }
.spark .area { fill-opacity: 0.12 }
.spark .line { stroke-width: 1.75 }
.spark .point.quiet { opacity: 0 }
.spark .point.quiet:hover, .spark .point.quiet:focus-visible { opacity: 1 }

/* A sunburst's outer ring is its inner ring's colour, softened, so its names
   are set in ink where the inner ring's are set in white. */
.slice.child { fill-opacity: 0.58 }
.sun-label { font-size: 10px; font-weight: 600; fill: #fff; pointer-events: none; text-anchor: middle }
.sun-label.outer { fill: var(--cr-ink); paint-order: stroke; stroke: var(--cr-surface); stroke-width: 2px }
.web .tick { paint-order: stroke; stroke: var(--cr-surface); stroke-width: 3px; stroke-linejoin: round }
.marks:hover .radar:not(:hover) { opacity: 0.4 }

/* Pointing at one bar lets the rest fall back, so the one being read stands
   out without a colour that is not in the data. */
.fill, .col, .dot, .slice, .cell, .tree-cell, .band, .radar, .box { transition: opacity 0.15s, transform 0.15s }
.marks:hover .fill:not(:hover), .marks:hover .col:not(:hover) { opacity: 0.55 }
.dot { stroke: var(--cr-surface); stroke-width: 1.5; fill-opacity: 0.85 }
.dot:hover { fill-opacity: 1 }
.dot:focus-visible, .pin:focus-visible, .fill:focus-visible, .col:focus-visible, .slice:focus-visible,
.cell:focus-visible, .tree-cell:focus-visible, .band:focus-visible,
.geo path:focus-visible, .geo-above path:focus-visible, .routes g:focus-visible {
  outline: 2px solid var(--cr-accent);
  outline-offset: 2px;
}
.pin {
  /* A ring in the surface colour, so two pins that overlap still read as
     two. Without it a cluster is one blob. */
  stroke: var(--cr-surface);
  stroke-width: 1.2;
  vector-effect: non-scaling-stroke;
}

/* A pie beside its list of slices, or above it where the panel is narrow. */
.pie-wrap { display: flex; align-items: center; gap: 20px }
.pie { display: block; width: min(200px, 100%); height: auto; flex: none; overflow: visible }
.slice { stroke: var(--cr-surface); stroke-width: 1.5; cursor: default }
.slices { list-style: none; margin: 0; padding: 0; display: grid; gap: 2px; flex: 1; min-width: 0 }
.slices li { display: grid; grid-template-columns: auto 1fr auto auto; gap: 8px; align-items: center; font-size: 12px; padding: 3px 6px; border-radius: 6px }
.slices li.on { background: color-mix(in srgb, var(--cr-line) 55%, transparent) }
.slices .name { color: var(--cr-ink-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap }
.slices .v, .slices .share { font-variant-numeric: tabular-nums }
.slices .share { color: var(--cr-ink-muted); min-width: 3.2em; text-align: right }
@container (max-width: 380px) {
  .pie-wrap { flex-direction: column; align-items: stretch }
  .pie { margin: 0 auto }
}
.centre { font-size: 22px; font-weight: 650; letter-spacing: -0.02em; fill: var(--cr-ink); text-anchor: middle; font-variant-numeric: tabular-nums }
.centre-label { font-size: 12px; fill: var(--cr-ink-muted); text-anchor: middle }

/* Marks arrive rather than appear, the first time a chart is drawn: columns
   rise, bars reach, lines draw themselves, the rest fade in. Never again on a
   resize, and never for a reader who asked their system for less motion. */
@keyframes cr-rise { from { transform: scaleY(0) } }
@keyframes cr-reach { from { transform: scaleX(0) } }
@keyframes cr-draw { from { stroke-dashoffset: 1 } }
@keyframes cr-fade { from { opacity: 0 } }
.enter .col { transform-box: fill-box; transform-origin: 50% 100%; animation: cr-rise 0.55s cubic-bezier(0.2, 0.7, 0.2, 1) both }
.enter .fill { transform-box: fill-box; transform-origin: 0 50%; animation: cr-reach 0.55s cubic-bezier(0.2, 0.7, 0.2, 1) both }
.enter .line, .enter .reading { stroke-dasharray: 1; animation: cr-draw 0.9s cubic-bezier(0.3, 0.6, 0.2, 1) both }
.enter .area, .enter .point, .enter .dot, .enter .slice, .enter .cell, .enter .cell-value, .enter .tree-cell,
.enter .band, .enter .neck, .enter .labels, .enter .value, .enter .shade, .enter .target, .enter .radar,
.enter .box, .enter .flow-band, .enter .flow-node {
  animation: cr-fade 0.6s ease-out both }

/* -- Maps --------------------------------------------------------------- */

/* The window onto the map. Its own class, where it used to share .stage with
   the funnel's rows — and so a grid display and a gap it never asked for.

   The data's proportions come in as --geo-aspect, capped in height: a
   full-width map at the data's shape is most of a screen tall, and a reader
   scrolling past it should not have to scroll through it. One finger pans the
   page, never the map (touch-action), so a phone can still scroll past; the
   map takes two — see map/controls.ts. */
.geo-stage {
  position: relative;
  width: 100%;
  aspect-ratio: var(--geo-aspect, 1.6);
  max-height: 520px;
  margin-top: 4px;
  overflow: hidden;
  border-radius: 4px;
  /* The ground under a map with no basemap, and past the tiles' edge. */
  background: color-mix(in srgb, var(--cr-line) 45%, var(--cr-surface));
  cursor: grab;
  outline: none;
  touch-action: pan-x pan-y;
  user-select: none;
  -webkit-user-select: none;
}
.geo-stage:focus-visible { box-shadow: inset 0 0 0 2px var(--cr-accent) }
.geo-stage.dragging { cursor: grabbing }
/* No stroke unless a layer asks for one. Stroke inherits, and the report's
   own .grid container sets one — one unit wide, which on a map is the width
   of the planet. Every heat disc drew it, and the density layer painted as a
   grey sheet over the whole map. */
.geo { position: absolute; inset: 0; display: block; width: 100%; height: 100%; stroke: none }
/* What is drawn over a map — other datasets, the area the report is narrowed
   to — above the painted places, which would otherwise cover it. The pointer
   passes through everywhere but the marks. */
.geo-above {
  position: absolute; inset: 0; display: block; width: 100%; height: 100%;
  stroke: none; pointer-events: none;
}
.geo-above > g { pointer-events: visiblePainted }
/* The hairline between two shaded areas. Areas only: written as ".geo path"
   it outranked the flow and route rules and drew both in the surface colour
   at 0.6px — a white line on a white map. */
.shapes path, .hexes path { stroke: var(--cr-surface); stroke-width: 0.6; vector-effect: non-scaling-stroke }
/* A ring inside another is a hole whichever way either was wound. GeoJSON asks
   for opposite windings and data does not always keep to it; paper decides
   holes by containment (run/printmap.go), and the screen has to agree. */
.shapes path { fill-rule: evenodd }
/* Shading over a basemap lets the streets through; on a bare map there is
   nothing under it to see. */
.tiled .shapes path { fill-opacity: 0.62 }
.tiled .hexes path { fill-opacity: 0.8 }
.tiles, .tile-level, .tiles-theme { position: absolute; inset: 0; pointer-events: none }
.tiles img { position: absolute; display: block; max-width: none; user-select: none; -webkit-user-drag: none }
/* A dataset drawn over a map sets --cr-pin on its layers, so its places are
   not the map's own colour. */
.pin { fill: var(--cr-pin, var(--cr-series-2)) }
.flow { fill: none; stroke-opacity: 0.8; stroke-linecap: round; vector-effect: non-scaling-stroke }
/* A flow's head, the flow's own colour; it says which way, and takes no
   pointer from the line it ends. */
.flow-head { fill-opacity: 0.9; pointer-events: none }
/* Flows that move, from where each starts to where it lands, when the author
   asks — and still for a reader who has asked their system for less motion. */
.flows.moving .flow { stroke-dasharray: 8 6; animation: cr-flow 1s linear infinite }
@keyframes cr-flow { to { stroke-dashoffset: -14 } }
@media (prefers-reduced-motion: reduce) {
  .flows.moving .flow { animation: none; stroke-dasharray: none }
}
/* A radius around a place, as a wash under its edge: where two catchments
   overlap reads as overlap. The pin colour, so an overlay's tint and a
   category's colour reach it as they reach the places. */
.ring {
  fill: var(--cr-pin, var(--cr-series-2));
  fill-opacity: 0.12;
  stroke: var(--cr-pin, var(--cr-series-2));
  stroke-opacity: 0.55;
  stroke-width: 1.5;
  vector-effect: non-scaling-stroke;
}
/* Names on the map, over everything drawn on it and under the tools. A halo
   in the surface colour keeps a name readable over any street or shade. */
.geo-labels { position: absolute; inset: 0; pointer-events: none; overflow: hidden }
.geo-label {
  position: absolute;
  left: 0;
  top: -7px;
  font-size: 11px;
  font-weight: 600;
  line-height: 14px;
  white-space: nowrap;
  color: var(--cr-ink);
  text-shadow: 0 0 2px var(--cr-surface), 0 0 2px var(--cr-surface), 0 0 3px var(--cr-surface);
}
.geo-label:not(.beside) { translate: -50% 0 }
.routes path { fill: none; stroke-linecap: round; stroke-linejoin: round; vector-effect: non-scaling-stroke }
.routes .casing { stroke: var(--cr-surface); stroke-width: 7; stroke-opacity: 0.9 }
.routes .route { stroke-width: 3.5 }
.routes g:hover .route, .routes g:focus-visible .route { stroke-width: 5.5 }
.heat { pointer-events: none }
/* A region or place that sets the report's filter when clicked, and the one
   it is set to. */
.pickable { cursor: pointer }
.shapes path.picked, .pin.picked {
  stroke: var(--cr-accent);
  stroke-width: 2.5;
  vector-effect: non-scaling-stroke;
}
/* The part of the world the report is narrowed to. Not .area, which is a
   line chart's fill, and in the same sheet. */
.geo-area {
  fill: none;
  stroke: var(--cr-accent);
  stroke-width: 2;
  stroke-dasharray: 6 4;
  vector-effect: non-scaling-stroke;
  pointer-events: none;
}
.geo-tools { position: absolute; top: 8px; left: 8px; display: flex; gap: 6px }
.geo-tools button {
  padding: 5px 10px;
  border: 0;
  border-radius: 6px;
  font: inherit;
  font-size: 12px;
  color: var(--cr-ink);
  background: var(--cr-surface);
  box-shadow: 0 1px 4px rgb(0 0 0 / 0.2);
  cursor: pointer;
}
.geo-tools button:hover { background: color-mix(in srgb, var(--cr-line) 50%, var(--cr-surface)) }
.geo-tools button:focus-visible { outline: 2px solid var(--cr-accent); outline-offset: 2px }
.legend-title { margin: 10px 0 0; font-size: 12px; font-weight: 600; color: var(--cr-ink-secondary) }
.area-none { font-size: 12px; color: var(--cr-ink-muted) }
.area-said { font-size: 12px; font-variant-numeric: tabular-nums; color: var(--cr-ink) }
/* A large map's places, painted over the shapes. The pointer passes through
   to the map and the shapes; the painted marks answer it by position. */
.geo-canvas { position: absolute; inset: 0; width: 100%; height: 100%; pointer-events: none }
/* A view on its way: a thread along the top, rather than a spinner over the
   map a reader is still looking at. */
.geo-stage.busy::after {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  width: 30%;
  height: 2px;
  background: var(--cr-accent);
  animation: geo-busy 1.1s linear infinite;
}
@keyframes geo-busy { from { transform: translateX(-100%) } to { transform: translateX(340%) } }
@media (prefers-reduced-motion: reduce) { .geo-stage.busy::after { animation: none; width: 100% } }

/* Counts over the map, in HTML so they are text at the page's size and
   buttons a keyboard can reach. The layer passes the pointer through to the
   map; the counts take it back. */
.geo-overlay { position: absolute; inset: 0; pointer-events: none; overflow: hidden }
.cluster {
  position: absolute;
  left: 0;
  top: 0;
  display: grid;
  place-items: center;
  padding: 0;
  border: 2px solid var(--cr-surface);
  border-radius: 999px;
  background: var(--cr-accent);
  color: #fff;
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  box-shadow: 0 1px 4px rgb(0 0 0 / 0.25);
  pointer-events: auto;
  cursor: zoom-in;
}
.cluster:focus-visible { outline: 2px solid var(--cr-accent); outline-offset: 2px }

.geo-controls {
  position: absolute;
  top: 8px;
  right: 8px;
  display: grid;
  gap: 1px;
  border-radius: 6px;
  overflow: hidden;
  background: var(--cr-line);
  box-shadow: 0 1px 4px rgb(0 0 0 / 0.2);
}
.geo-controls button {
  width: 30px;
  height: 30px;
  padding: 0;
  border: 0;
  font: inherit;
  font-size: 17px;
  line-height: 1;
  color: var(--cr-ink);
  background: var(--cr-surface);
  cursor: pointer;
}
.geo-controls button:hover:not(:disabled) { background: color-mix(in srgb, var(--cr-line) 50%, var(--cr-surface)) }
.geo-controls button:disabled { color: var(--cr-ink-muted); opacity: 0.55; cursor: default }
.geo-controls button:focus-visible { outline: 2px solid var(--cr-accent); outline-offset: -2px }
.geo-hint {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  padding: 8px 12px;
  border-radius: 6px;
  background: rgb(0 0 0 / 0.72);
  color: #fff;
  font-size: 13px;
  white-space: nowrap;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.2s;
}
.geo-hint.on { opacity: 1 }
/* Where the providers' own libraries put it, at the height their terms ask
   for — and never restyled, which both of those terms forbid. */
.geo-logo { position: absolute; left: 8px; bottom: 6px; height: 20px; width: auto; pointer-events: none }
.credit { margin-top: 6px; font-size: 11px; color: var(--cr-ink-muted) }
.credit a { color: inherit; text-decoration: underline; text-decoration-color: color-mix(in srgb, currentColor 40%, transparent) }
.credit .note { display: block; margin-top: 2px; color: var(--cr-ink-secondary) }
.swatch.dot, .swatch.circle { border-radius: 999px }
/* A category's glyph in its key, as its places are drawn — see glyphs.ts. */
.swatch.square { border-radius: 1px }
.swatch.triangle { border-radius: 0; clip-path: polygon(50% 0, 100% 100%, 0 100%) }
/* What a bubble's size says: circles drawn to the bubbles' own rule. */
.legend.sizes { align-items: flex-end }
.size-ring { display: block; flex: none; overflow: visible }
.size-ring circle { fill: none; stroke: var(--cr-ink-muted); stroke-width: 1 }
/* A region with nothing in the period a map is playing: still on the map,
   empty, its edge where it was. A rule over the fill attribute each period
   sets, so the step it would have had does not show through. */
.shapes path.void { fill: var(--cr-line); fill-opacity: 0.35 }
/* A period's shades ease into the next, for a reader who has not asked their
   system for less motion. Nothing else changes a shade once it is drawn. */
@media (prefers-reduced-motion: no-preference) {
  .shapes path { transition: fill 0.35s }
  .routes .route { transition: stroke 0.35s }
}
/* The periods under a map that plays through time: play, a slider, the one
   shown, and every period together. */
.geo-time {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 8px;
  font-size: 12px;
  color: var(--cr-ink-secondary);
}
.geo-time button {
  flex: none;
  height: 28px;
  padding: 0 10px;
  border: 1px solid var(--cr-line);
  border-radius: 6px;
  font: inherit;
  color: var(--cr-ink);
  background: var(--cr-surface);
  cursor: pointer;
}
.geo-time .play { display: grid; place-items: center; width: 32px; padding: 0 }
.geo-time .play path { fill: currentColor }
.geo-time button:hover { background: color-mix(in srgb, var(--cr-line) 50%, var(--cr-surface)) }
.geo-time button:focus-visible, .geo-time input:focus-visible { outline: 2px solid var(--cr-accent); outline-offset: 2px }
.geo-time .all[aria-pressed='true'] { border-color: var(--cr-accent); color: var(--cr-accent) }
.geo-time input { flex: 1; min-width: 80px; margin: 0; accent-color: var(--cr-accent) }
/* Every period together: the slider waits to be moved, not on its first stop. */
.geo-time input.idle { opacity: 0.45 }
.geo-time .period { flex: none; min-width: 9ch; font-variant-numeric: tabular-nums; color: var(--cr-ink) }

/* -- Legend and tooltip -------------------------------------------------- */

/* Always present past one series: colour is the only thing telling series
   apart, and a colour with no name is a question. The label wears ink rather
   than the series hue — coloured 12px text fails contrast on half the palette,
   and the swatch beside it already carries the identity. */
.legend { display: flex; flex-wrap: wrap; gap: 4px 12px; margin-top: 10px; font-size: 12px }
.key { display: inline-flex; align-items: center; gap: 6px; color: var(--cr-ink-secondary) }
.swatch { width: 10px; height: 10px; border-radius: 2px; flex: none }
/* A key that hides its series: pressed while the series shows. */
button.key { font: inherit; color: var(--cr-ink-secondary); background: none; border: 0; padding: 2px 6px; margin: 0 -6px; border-radius: 6px; cursor: pointer }
button.key:hover { background: color-mix(in srgb, var(--cr-line) 55%, transparent) }
button.key:focus-visible { outline: 2px solid var(--cr-accent); outline-offset: 1px }
button.key[aria-pressed='false'] { color: var(--cr-ink-muted); text-decoration: line-through }
button.key[aria-pressed='false'] .swatch { background: transparent !important; box-shadow: inset 0 0 0 1.5px var(--cr-ink-muted) }
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
/* Every series at one point, each beside its colour and its number. */
.tip .tip-row { display: flex; align-items: center; gap: 6px; opacity: 1 }
.tip .tip-row em { margin-left: auto; padding-left: 14px; font-style: normal; font-weight: 600 }

/* Motion is a preference, and a report is not the place to override it. */
@media (prefers-reduced-motion: reduce) {
  * { transition: none !important; animation: none !important }
}

/* -- Combo, funnel, waterfall ------------------------------------------- */

.rule-swatch { height: 3px; border-radius: 2px }
.neck { opacity: 0.16 }

/* -- Heatmap ------------------------------------------------------------- */

/* A pair no row matched: an outline rather than the ramp's lightest step,
   because "none" and "nearly none" must not look alike. */
.cell.none { fill: transparent; stroke: var(--cr-line) }
/* A cell's number in the ink its shade calls for — see --cr-on-ramp-*, which
   switch with the shades whichever theme is on. */
.cell-value { font-size: 11px; font-weight: 550; text-anchor: middle; pointer-events: none; font-variant-numeric: tabular-nums }
.cell-value.s1 { fill: var(--cr-on-ramp-1) }
.cell-value.s2 { fill: var(--cr-on-ramp-2) }
.cell-value.s3 { fill: var(--cr-on-ramp-3) }
.cell-value.s4 { fill: var(--cr-on-ramp-4) }
.cell-value.s5 { fill: var(--cr-on-ramp-5) }
.cell-value.s6 { fill: var(--cr-on-ramp-6) }

/* -- Gauge --------------------------------------------------------------- */

.gauge { display: block; width: min(240px, 100%); height: auto; margin: 0 auto; overflow: visible }
.gauge .track, .gauge .reading { fill: none; stroke-width: 16; stroke-linecap: round }
.gauge .track { stroke: var(--cr-line) }
.gauge .reading { stroke: var(--cr-accent) }
.gauge .reading.over { stroke: var(--cr-good) }
.gauge .centre { font-size: 26px }
.gauge .tick { text-anchor: middle }
.gauge-note { text-align: center }

/* -- Treemap ------------------------------------------------------------- */

.tree-frame { fill: none; stroke: var(--cr-line) }
.tree-group { font-size: 10px; font-weight: 650; letter-spacing: 0.05em; text-transform: uppercase; fill: var(--cr-ink-muted) }
.tree-cell { cursor: default }
.tree-cell:hover { opacity: 0.88 }
/* White on every step of both ramps, with a halo for the lightest: the ramps
   clear their contrast floors against the surface, not against text laid on
   them. */
.tree-label, .tree-value { fill: #fff; pointer-events: none; paint-order: stroke; stroke: rgb(0 0 0 / 0.3); stroke-width: 2px; stroke-linejoin: round }
.tree-label { font-size: 12px; font-weight: 600 }
.tree-value { font-size: 11px; opacity: 0.92; font-variant-numeric: tabular-nums }
@media (prefers-color-scheme: dark) {
  $SCOPE$ {
${DARK}  }
}
/* A theme the page names beats the reader's system setting, as the maps'
   basemaps already did: the portal's own switch, or data-theme set on the
   element. It used to follow the system alone, so a reader who picked the
   dark theme on a light system got ramps stepped for the wrong surface. */
$DARK$ {
${DARK}}
$LIGHT$ {
${LIGHT}}
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
  // In a shadow root the element's own attribute names the theme; in a
  // document, the attribute on any ancestor — the portal's <html> — or on
  // the chart itself.
  const named = (theme: string) => scope === ':host'
    ? `:host([data-theme='${theme}'])`
    : `[data-theme='${theme}'] ${scope}, ${scope}[data-theme='${theme}']`
  return sheet.replaceAll('$DARK$', named('dark')).replaceAll('$LIGHT$', named('light')).replaceAll('$SCOPE$', scope)
}

/**
 * The cascade layer the document stylesheet declares itself in.
 *
 * Exported because a host has to name it in its own `@layer` statement, ahead
 * of the layers it keeps its own styles in. A layer nobody ordered is appended
 * last and therefore wins, which is the whole problem below.
 */
export const LAYER = 'cronos-charts'

/**
 * The same stylesheet, for an ordinary document rather than a shadow root.
 *
 * Inside a shadow root the boundary does the scoping, so the rules above are
 * written as plain class selectors — `.grid`, `.panel`, `.bars`, and a `*`
 * that sets box-sizing. Adopted into a document those selectors are global,
 * and an adopted stylesheet is unlayered, so each of them beats every layered
 * utility on the page.
 *
 * The portal's application shell is a `<div class="grid ...">`. The moment a
 * report drew its first chart, `.grid` gave that div
 * `repeat(auto-fit, minmax(220px, 1fr))`: the 248px navigation column became
 * half the window and the report was squeezed into the other half, for the
 * rest of the session. The same rule flattened the report's own block grid
 * into a masonry of orphaned cards, and `$SCOPE$`'s literal
 * `--cr-surface: #fff` beat the portal's themed one, so every chart stayed
 * white when the page went dark.
 *
 * Two wrappers, one per half of that:
 *
 *   - `@scope` stops the class selectors matching anything outside a chart, so
 *     a name as ordinary as `.grid` cannot reach the shell;
 *   - `@layer` puts the sheet below the host's own styles, so the host's
 *     `--cr-*` values win and the charts wear its theme.
 *
 * `withScope` is false only where `@scope` is not implemented, because an
 * at-rule a browser does not know is dropped with everything inside it — see
 * ServerChart. The layer alone still fixes the theming and everything the host
 * sets explicitly, which is the better half of two bad options.
 */
export function documentCss(scope: string, withScope = true): string {
  const rules = css(withScope ? ':scope' : scope)
  return `@layer ${LAYER} {\n${withScope ? `@scope (${scope}) {\n${rules}\n}` : rules}\n}\n`
}
