/**
 * TradingView Charting Library TypeScript Declarations
 * Minimal type definitions for the Tragge trading platform integration.
 */

declare namespace TradingView {
  interface ChartingLibraryWidgetOptions {
    symbol: string;
    datafeed: IBasicDataFeed;
    interval: string;
    container: HTMLElement | string;
    library_path: string;
    locale?: string;
    disabled_features?: string[];
    enabled_features?: string[];
    fullscreen?: boolean;
    autosize?: boolean;
    theme?: 'Light' | 'Dark';
    overrides?: Record<string, unknown>;
    studies_overrides?: Record<string, unknown>;
    loading_screen?: {
      backgroundColor: string;
      foregroundColor: string;
    };
    custom_css_url?: string;
    timezone?: string;
    debug?: boolean;
    time_frames?: TimeFrameItem[];
    charts_storage_url?: string;
    charts_storage_api_version?: '1.0' | '1.1';
    client_id?: string;
    user_id?: string;
    favorites?: {
      intervals?: string[];
      chartTypes?: string[];
    };
    save_load_adapter?: SaveLoadAdapter;
  }

  interface TimeFrameItem {
    text: string;
    resolution: string;
    description?: string;
    title?: string;
  }

  interface SaveLoadAdapter {
    getAllCharts(): Promise<ChartMetaInfo[]>;
    removeChart(id: string): Promise<void>;
    saveChart(chartData: ChartData): Promise<string>;
    getChartContent(id: string): Promise<string>;
    getAllStudyTemplates(): Promise<StudyTemplateMetaInfo[]>;
    removeStudyTemplate(studyTemplateInfo: StudyTemplateMetaInfo): Promise<void>;
    saveStudyTemplate(studyTemplateData: StudyTemplateData): Promise<void>;
    getStudyTemplateContent(studyTemplateInfo: StudyTemplateMetaInfo): Promise<string>;
  }

  interface ChartMetaInfo {
    id: string;
    name: string;
    symbol: string;
    resolution: string;
    timestamp: number;
  }

  interface ChartData {
    id?: string;
    name: string;
    symbol: string;
    resolution: string;
    content: string;
  }

  interface StudyTemplateMetaInfo {
    name: string;
  }

  interface StudyTemplateData {
    name: string;
    content: string;
  }

  interface IChartingLibraryWidget {
    onChartReady(callback: () => void): void;
    activeChart(): IChartWidgetApi;
    setSymbol(symbol: string, interval: string, callback?: () => void): void;
    remove(): void;
    chart(index?: number): IChartWidgetApi;
    save(callback: (state: object) => void): void;
    load(state: object): void;
    subscribe(event: string, callback: () => void): void;
    unsubscribe(event: string, callback: () => void): void;
  }

  interface IChartWidgetApi {
    setSymbol(symbol: string, interval?: string): void;
    symbol(): string;
    resolution(): string;
    setResolution(resolution: string, callback?: () => void): void;
    resetData(): void;
    executeActionById(actionId: string): void;
    getVisibleRange(): VisibleRange;
    setVisibleRange(range: VisibleRange, callback?: () => void): void;
    onSymbolChanged(): ISubscription<() => void>;
    onIntervalChanged(): ISubscription<() => void>;
    createStudy(
      name: string,
      forceOverlay: boolean,
      lock?: boolean,
      inputs?: StudyInputs,
      callback?: (entityId: string) => void,
      overrides?: StudyOverrides,
      options?: CreateStudyOptions
    ): Promise<string | null>;
    removeAllStudies(): void;
    crossHairMoved(callback: (params: CrossHairMovedParams) => void): void;
  }

  interface VisibleRange {
    from: number;
    to: number;
  }

  interface ISubscription<T> {
    subscribe(callback: T, context?: object, singleShot?: boolean): void;
    unsubscribe(callback: T): void;
    unsubscribeAll(): void;
  }

  interface StudyInputs {
    [key: string]: string | number | boolean;
  }

  interface StudyOverrides {
    [key: string]: string | number | boolean;
  }

  interface CreateStudyOptions {
    checkLimit?: boolean;
    priceAxisRange?: string;
  }

  interface CrossHairMovedParams {
    time?: number;
    price?: number;
  }

  // Datafeed interfaces
  interface IBasicDataFeed {
    onReady(callback: (config: DatafeedConfiguration) => void): void;
    searchSymbols(
      userInput: string,
      exchange: string,
      symbolType: string,
      onResult: SearchSymbolsCallback
    ): void;
    resolveSymbol(
      symbolName: string,
      onResolve: ResolveCallback,
      onError: ErrorCallback
    ): void;
    getBars(
      symbolInfo: LibrarySymbolInfo,
      resolution: string,
      periodParams: PeriodParams,
      onResult: HistoryCallback,
      onError: ErrorCallback
    ): void;
    subscribeBars(
      symbolInfo: LibrarySymbolInfo,
      resolution: string,
      onRealtimeCallback: SubscribeBarsCallback,
      subscriberUID: string,
      onResetCacheNeededCallback?: () => void
    ): void;
    unsubscribeBars(subscriberUID: string): void;
  }

  interface DatafeedConfiguration {
    supported_resolutions: string[];
    exchanges?: Exchange[];
    symbols_types?: DatafeedSymbolType[];
    supports_marks?: boolean;
    supports_timescale_marks?: boolean;
    supports_time?: boolean;
    currency_codes?: string[];
  }

  interface Exchange {
    value: string;
    name: string;
    desc: string;
  }

  interface DatafeedSymbolType {
    name: string;
    value: string;
  }

  interface LibrarySymbolInfo {
    name: string;
    full_name: string;
    description: string;
    type: string;
    session: string;
    timezone: string;
    ticker: string;
    exchange: string;
    minmov: number;
    pricescale: number;
    has_intraday: boolean;
    has_daily: boolean;
    has_weekly_and_monthly: boolean;
    supported_resolutions: string[];
    volume_precision: number;
    data_status: string;
    format?: string;
  }

  interface PeriodParams {
    from: number;
    to: number;
    countBack?: number;
    firstDataRequest: boolean;
  }

  interface Bar {
    time: number;
    open: number;
    high: number;
    low: number;
    close: number;
    volume?: number;
  }

  type SearchSymbolsCallback = (symbols: SearchSymbolResult[]) => void;
  type ResolveCallback = (symbolInfo: LibrarySymbolInfo) => void;
  type ErrorCallback = (reason: string) => void;
  type HistoryCallback = (bars: Bar[], meta: HistoryMeta) => void;
  type SubscribeBarsCallback = (bar: Bar) => void;

  interface SearchSymbolResult {
    symbol: string;
    full_name: string;
    description: string;
    exchange: string;
    type: string;
  }

  interface HistoryMeta {
    noData?: boolean;
    nextTime?: number;
  }

  class widget implements IChartingLibraryWidget {
    constructor(options: ChartingLibraryWidgetOptions);
    onChartReady(callback: () => void): void;
    activeChart(): IChartWidgetApi;
    setSymbol(symbol: string, interval: string, callback?: () => void): void;
    remove(): void;
    chart(index?: number): IChartWidgetApi;
    save(callback: (state: object) => void): void;
    load(state: object): void;
    subscribe(event: string, callback: () => void): void;
    unsubscribe(event: string, callback: () => void): void;
  }
}

interface Window {
  TradingView: typeof TradingView;
}

export = TradingView;
export as namespace TradingView;
