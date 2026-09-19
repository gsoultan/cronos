import { statBlock } from './blocks/stat'
import { barBlock } from './blocks/bar'
import { lineBlock } from './blocks/line'
import { pieBlock } from './blocks/pie'
import { scatterBlock } from './blocks/scatter'
import { mapBlock } from './blocks/map'
import { comboBlock } from './blocks/combo'
import { funnelBlock } from './blocks/funnel'
import { waterfallBlock } from './blocks/waterfall'
import { heatmapBlock } from './blocks/heatmap'
import { gaugeBlock } from './blocks/gauge'
import { treemapBlock } from './blocks/treemap'
import { tableBlock } from './blocks/table'
import { textBlock } from './blocks/text'
import { unsupported } from './blocks/unsupported'
import type { Block, ChartBlock } from './types'

/**
 * The chart renderers, as plain DOM.
 *
 * Framework-free and dependency-free on purpose. There were three renderers of
 * the same payload — the embed, the paginated typesetter, and the portal — and
 * the third drew one chart type out of fourteen, answering "line charts need a
 * newer portal" for a report the server had rendered perfectly. Three
 * implementations is how that happens: two of them get the attention and the
 * quiet one drifts.
 *
 * The typesetter cannot share this (it places marks the server arranged; see
 * internal/core/document/mark.go), but the two that draw in a browser can, and
 * now do.
 */
export function drawBlock(b: Block): HTMLElement {
  switch (b.kind) {
    case 'stat':
      return statBlock(b)
    case 'chart':
      return drawChart(b)
    case 'table':
      return tableBlock(b)
    case 'text':
      return textBlock(b)
    default:
      // A block kind a host's pinned copy has never heard of is a normal
      // condition, not a bug: the server and the viewers ship separately.
      return unsupported('This block needs a newer viewer')
  }
}

/**
 * The chart type is an open set the server grows, so this switch has a default
 * for the same reason the one above does.
 */
export function drawChart(b: ChartBlock): HTMLElement {
  switch (b.chart) {
    case 'bar':
      return barBlock(b)
    case 'line':
      return lineBlock(b, false)
    case 'area':
      return lineBlock(b, true)
    case 'pie':
      return pieBlock(b, false)
    case 'donut':
      return pieBlock(b, true)
    case 'scatter':
    case 'bubble':
      return scatterBlock(b)
    case 'map':
      return mapBlock(b)
    case 'combo':
      return comboBlock(b)
    case 'funnel':
      return funnelBlock(b)
    case 'waterfall':
      return waterfallBlock(b)
    case 'heatmap':
      return heatmapBlock(b)
    case 'gauge':
      return gaugeBlock(b)
    case 'treemap':
      return treemapBlock(b)
    default:
      return unsupported(`${b.chart} charts need a newer viewer`)
  }
}

export { filterBar } from './filters'
export { unaffectedNote } from './coverage'
export { el, fill } from './dom'
export { css } from './styles'
export type {
  Arc, Axis, Bar, Block, Bounds, Cell, ChartBlock, Coverage, Delta, FilterDef,
  FilterValues, Gauge, GeoMap, Group, LegendStop, Marker, Point, Rect,
  ReportPayload, Shape, Stage, StatBlock, Step, TableBlock, TextBlock, Tick, Tiles, Track,
} from './types'
