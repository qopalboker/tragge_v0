// Structured logging with environment-aware levels and mandatory redaction.

type LogLevel = 'debug' | 'info' | 'warn' | 'error';

interface LoggerConfig {
  enabled: boolean;
  level: LogLevel;
  prefix: string;
}

export const REDACTED_VALUE = '[REDACTED]';

const LOG_LEVELS: Record<LogLevel, number> = { debug: 0, info: 1, warn: 2, error: 3 };
const SENSITIVE_KEY = /(authorization|cookie|password|passwd|passphrase|token|jwt|secret|api[_-]?key|private[_-]?key|signature|otp|code|grant|ticket|request[_-]?body|response[_-]?body|payment[_-]?payload|kyc|document[_-]?bytes|email|phone|national|iban|card[_-]?number|account[_-]?number)/i;

export function redactTextForLogging(value: string): string {
  return value
    .replace(/-----BEGIN [^-\r\n]*PRIVATE KEY-----[\s\S]*?-----END [^-\r\n]*PRIVATE KEY-----/g, REDACTED_VALUE)
    .replace(/\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}(?:\.[A-Za-z0-9_-]{8,})?\b/g, REDACTED_VALUE)
    .replace(/\b(bearer|basic)\s+[^\s,;]+/gi, `$1 ${REDACTED_VALUE}`)
    .replace(/\b((?:postgres(?:ql)?|redis(?:s)?|https?):\/\/)[^\s/@]*(?::[^\s/@]*)?@/gi, `$1${REDACTED_VALUE}@`)
    .replace(/\b(authorization|cookie|password|passwd|passphrase|access[_-]?token|refresh[_-]?token|session[_-]?token|jwt|api[_-]?key|client[_-]?secret|private[_-]?key|provider[_-]?secret|webhook[_-]?secret|webhook[_-]?signature|csrf[_-]?token|otp|verification[_-]?code|reset[_-]?code|reset[_-]?token|security[_-]?code|national[_-]?code|reauth(?:entication)?[_-]?grant|ticket)\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;&}]+)/gi, `$1=${REDACTED_VALUE}`);
}

export function redactForLogging(value: unknown, depth = 0, seen = new WeakSet<object>()): unknown {
  if (value === null || value === undefined || typeof value === 'boolean' || typeof value === 'number') return value;
  if (typeof value === 'string') return redactTextForLogging(value);
  if (typeof value === 'bigint' || typeof value === 'symbol' || typeof value === 'function') return String(value);
  if (depth > 12) return REDACTED_VALUE;

  if (value instanceof Error) {
    return {
      name: value.name,
      message: redactTextForLogging(value.message),
      stack: value.stack ? redactTextForLogging(value.stack) : undefined,
    };
  }
  if (value instanceof URL) return redactTextForLogging(value.toString());
  if (typeof Blob !== 'undefined' && value instanceof Blob) return REDACTED_VALUE;
  if (typeof Headers !== 'undefined' && value instanceof Headers) return redactEntries(Array.from(value.entries()), depth, seen);
  if (value instanceof URLSearchParams) return redactEntries(Array.from(value.entries()), depth, seen);
  if (typeof FormData !== 'undefined' && value instanceof FormData) return redactEntries(Array.from(value.entries()), depth, seen);
  if (Array.isArray(value)) return value.map((item) => redactForLogging(item, depth + 1, seen));
  if (value instanceof Date) return value.toISOString();

  if (typeof value === 'object') {
    if (seen.has(value)) return REDACTED_VALUE;
    seen.add(value);
    return redactEntries(Object.entries(value as Record<string, unknown>), depth, seen);
  }
  return REDACTED_VALUE;
}

function redactEntries(entries: Array<[string, unknown]>, depth: number, seen: WeakSet<object>): Record<string, unknown> {
  return Object.fromEntries(entries.map(([key, item]) => [
    key,
    SENSITIVE_KEY.test(key) ? REDACTED_VALUE : redactForLogging(item, depth + 1, seen),
  ]));
}

let consoleRedactionInstalled = false;

export function installConsoleRedaction(): void {
  if (consoleRedactionInstalled) return;
  consoleRedactionInstalled = true;
  for (const level of ['debug', 'info', 'warn', 'error'] as const) {
    const original = console[level].bind(console);
    console[level] = (...args: unknown[]) => original(...args.map((arg) => redactForLogging(arg)));
  }
}

class Logger {
  private config: LoggerConfig;

  constructor(config: Partial<LoggerConfig> = {}) {
    const envLogLevel = import.meta.env.VITE_LOG_LEVEL as LogLevel | undefined;
    const isDev = import.meta.env.DEV;
    this.config = {
      enabled: isDev || !!envLogLevel,
      level: envLogLevel || (isDev ? 'info' : 'error'),
      prefix: config.prefix || '',
    };
  }

  private shouldLog(level: LogLevel): boolean {
    return this.config.enabled && LOG_LEVELS[level] >= LOG_LEVELS[this.config.level];
  }

  private formatPrefix(): string { return this.config.prefix ? `[${this.config.prefix}]` : ''; }

  debug(...args: unknown[]): void { if (this.shouldLog('debug')) console.debug(this.formatPrefix(), ...args.map((arg) => redactForLogging(arg))); }
  info(...args: unknown[]): void { if (this.shouldLog('info')) console.info(this.formatPrefix(), ...args.map((arg) => redactForLogging(arg))); }
  warn(...args: unknown[]): void { if (this.shouldLog('warn')) console.warn(this.formatPrefix(), ...args.map((arg) => redactForLogging(arg))); }
  error(...args: unknown[]): void { if (this.shouldLog('error')) console.error(this.formatPrefix(), ...args.map((arg) => redactForLogging(arg))); }

  child(prefix: string): Logger {
    const childLogger = new Logger();
    childLogger.config = { ...this.config, prefix: this.config.prefix ? `${this.config.prefix}:${prefix}` : prefix };
    return childLogger;
  }
}

export const logger = new Logger();
