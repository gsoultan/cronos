import { statBlock } from './blocks/stat'
import { barBlock } from './blocks/bar'
import { columnBlock } from './blocks/column'
import { radarBlock } from './blocks/radar'
import { bulletBlock } from './blocks/bullet'
import { histogramBlock } from './blocks/histogram'
import { boxplotBlock } from './blocks/boxplot'
import { sankeyBlock } from './blocks/sankey'
import { sunburstBlock } from './blocks/sunburst'
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
import type { Block, ChartBlock, DrawOptions } from './types'

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
export function drawBlock(b: Block, opts: DrawOptions = {}): HTMLElement {
  switch (b.kind) {
    case 'stat':
      return statBlock(b)
    case 'chart':
      return drawChart(b, opts)
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
export function drawChart(b: ChartBlock, opts: DrawOptions = {}): HTMLElement {
  switch (b.chart) {
    case 'bar':
      return barBlock(b)
    case 'column':
      return columnBlock(b)
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
      return mapBlock(b, opts)
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
    case 'radar':
      return radarBlock(b)
    case 'bullet':
      return bulletBlock(b)
    case 'histogram':
      return histogramBlock(b)
    case 'boxplot':
      return boxplotBlock(b)
    case 'sankey':
      return sankeyBlock(b)
    case 'sunburst':
      return sunburstBlock(b)
    default:
      return unsupported(`${b.chart} charts need a newer viewer`)
  }
}

export { filterBar } from './filters'
export { describeArea } from './map/sets'
export { unaffectedNote } from './coverage'
export { el, fill } from './dom'
export { css, documentCss, LAYER } from './styles'
export type {
  Arc, Axis, Bar, Bin, Block, Bounds, Box, Bullet, Cell, Sankey, SankeyLink, SankeyNode, Cells, ChartBlock, Coverage, Credit, Delta, DrawOptions,
  FilterDef, FilterValues, FrameMark, Frames, Gauge, GeoMap, Group, LegendStop, MapArea, MapDetail, MapKey, MapPick,
  MapViewAsk, MapViewer, Marker, Point, Rect, ReportPayload, Shape, Stage, StatBlock, Step, TableBlock,
  TextBlock, Tick, Tiles, Track,
} from './types'
