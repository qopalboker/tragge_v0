/**
 * Drawing Tools integration with lightweight-charts-drawing plugin.
 *
 * This module re-exports the DrawingManager and key drawing classes from
 * the lightweight-charts-drawing package, plus provides persistence helpers
 * for saving/loading drawings per contest+symbol via localStorage.
 */
import {
  DrawingManager,
  TrendLine,
  HorizontalLine,
  VerticalLine,
  Rectangle,
  FibRetracement,
  TextAnnotation,
  Ray,
  ExtendedLine,
  InfoLine,
  TrendAngle,
  Arrow,
  HorizontalRay,
  CrossLine,
  ParallelChannel,
  RegressionTrend,
  FlatTopBottom,
  FibExtension,
  FibArcs,
  FibTimeZone,
  FibSpeedFan,
  GannBox,
  GannSquare,
  GannFan,
  AndrewsPitchfork,
  SchiffPitchfork,
  ModifiedSchiffPitchfork,
  InsidePitchfork,
  RotatedRectangle,
  Triangle,
  Circle,
  Ellipse,
  Arc,
  Polyline,
  Path,
  Callout,
  PriceLabel,
  Note,
  PriceRange,
  DateRange,
  DatePriceRange,
  Brush,
  Highlighter,
  getToolRegistry,
  type Anchor,
  type DrawingStyle,
  type DrawingOptions,
  type SerializedDrawing,
  type DrawingEventType,
  type DrawingCategory,
  type IDrawing,
} from 'lightweight-charts-drawing';

// Re-export everything consumers need
export {
  DrawingManager,
  TrendLine,
  HorizontalLine,
  VerticalLine,
  Rectangle,
  FibRetracement,
  TextAnnotation,
  Ray,
  ExtendedLine,
  InfoLine,
  TrendAngle,
  Arrow,
  HorizontalRay,
  CrossLine,
  ParallelChannel,
  RegressionTrend,
  FlatTopBottom,
  FibExtension,
  FibArcs,
  FibTimeZone,
  FibSpeedFan,
  GannBox,
  GannSquare,
  GannFan,
  AndrewsPitchfork,
  SchiffPitchfork,
  ModifiedSchiffPitchfork,
  InsidePitchfork,
  RotatedRectangle,
  Triangle,
  Circle,
  Ellipse,
  Arc,
  Polyline,
  Path,
  Callout,
  PriceLabel,
  Note,
  PriceRange,
  DateRange,
  DatePriceRange,
  Brush,
  Highlighter,
  getToolRegistry,
};

export type {
  Anchor,
  DrawingStyle,
  DrawingOptions,
  SerializedDrawing,
  DrawingEventType,
  DrawingCategory,
  IDrawing,
};

/**
 * Drawing tool categories for toolbar grouping
 */
export interface DrawingToolGroup {
  label: string;
  tools: DrawingToolInfo[];
}

export interface DrawingToolInfo {
  type: string;
  name: string;
  category: DrawingCategory;
}

/**
 * Get all available drawing tools grouped by category.
 * Returns the tool registry contents organized for the toolbar UI.
 */
export function getGroupedTools(): DrawingToolGroup[] {
  const registry = getToolRegistry();
  const categories: { label: string; category: DrawingCategory }[] = [
    { label: 'Trend', category: 'line' },
    { label: 'Channel', category: 'channel' },
    { label: 'Fibonacci', category: 'fibonacci' },
    { label: 'Gann', category: 'gann' },
    { label: 'Pitchfork', category: 'pitchfork' },
    { label: 'Shapes', category: 'shape' },
    { label: 'Annotations', category: 'annotation' },
    { label: 'Forecasting', category: 'forecasting' },
    { label: 'Measurement', category: 'measurement' },
  ];

  const groups: DrawingToolGroup[] = [];

  for (const cat of categories) {
    const tools = registry.getByCategory(cat.category);
    if (tools.length > 0) {
      groups.push({
        label: cat.label,
        tools: tools.map(t => ({
          type: t.type,
          name: t.name,
          category: t.category,
        })),
      });
    }
  }

  return groups;
}

/**
 * Quick-access tools shown as buttons in the toolbar
 */
export const QUICK_ACCESS_TOOLS = [
  'trend-line',
  'horizontal-line',
  'fib-retracement',
  'rectangle',
  'text-annotation',
] as const;

/**
 * LocalStorage key for drawing persistence
 */
function drawingsKey(contestId: string, symbol: string): string {
  return `tragge:drawings:${contestId}:${symbol}`;
}

// Debounce timers for server sync
const saveTimers = new Map<string, ReturnType<typeof setTimeout>>();
const SAVE_DEBOUNCE_MS = 2000;

/**
 * Save drawings — localStorage immediately + server debounced
 */
export function saveDrawings(
  manager: DrawingManager,
  contestId: string,
  symbol: string
): void {
  try {
    const data = manager.exportDrawings();
    const key = drawingsKey(contestId, symbol);
    const json = JSON.stringify(data);

    // localStorage immediately (fast cache)
    localStorage.setItem(key, json);

    // Server sync (debounced)
    const timerKey = `${contestId}:${symbol}`;
    const existing = saveTimers.get(timerKey);
    if (existing) clearTimeout(existing);

    saveTimers.set(timerKey, setTimeout(async () => {
      saveTimers.delete(timerKey);
      try {
        const { api } = await import('@/api');
        await api.put(
          `/api/trade/drawings/${contestId}?symbol=${encodeURIComponent(symbol)}`,
          { drawings: data }
        );
      } catch (err) {
        console.warn('Failed to sync drawings to server:', err);
      }
    }, SAVE_DEBOUNCE_MS));
  } catch (err) {
    console.warn('Failed to save drawings:', err);
  }
}

/**
 * Load drawings — localStorage first (fast), then server (authoritative)
 */
export async function loadDrawings(
  manager: DrawingManager,
  contestId: string,
  symbol: string
): Promise<void> {
  const registry = getToolRegistry();
  const importer = (type: string, serialized: SerializedDrawing) => {
    return registry.createDrawing(
      type,
      serialized.id,
      serialized.anchors,
      serialized.style,
      serialized.options
    );
  };

  // Phase 1: localStorage (fast UX)
  try {
    const cached = localStorage.getItem(drawingsKey(contestId, symbol));
    if (cached) {
      const data: SerializedDrawing[] = JSON.parse(cached);
      manager.importDrawings(data, importer);
    }
  } catch (err) {
    console.warn('Failed to load cached drawings:', err);
  }

  // Phase 2: Server (authoritative — replaces localStorage data)
  try {
    const { api } = await import('@/api');
    const response = await api.get<{ drawings: SerializedDrawing[] }>(
      `/api/trade/drawings/${contestId}?symbol=${encodeURIComponent(symbol)}`
    );
    if (response.data.drawings && response.data.drawings.length > 0) {
      manager.clearAll();
      manager.importDrawings(response.data.drawings, importer);
      // Update localStorage cache
      localStorage.setItem(
        drawingsKey(contestId, symbol),
        JSON.stringify(response.data.drawings)
      );
    }
  } catch (err) {
    // Server unavailable — localStorage fallback already loaded
    console.warn('Failed to load drawings from server:', err);
  }
}
