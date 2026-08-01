import { describe, it, expect } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';

const SRC_DIR = resolve(__dirname, '..');

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    const st = statSync(full);
    if (st.isDirectory()) {
      walk(full, out);
    } else if (/\.(ts|vue)$/.test(entry) && !/\.test\.ts$/.test(entry)) {
      out.push(full);
    }
  }
  return out;
}

function extractRouteNames(source: string): Set<string> {
  const names = new Set<string>();
  const re = /name:\s*'([a-z0-9-]+)'/gi;
  let m: RegExpExecArray | null;
  while ((m = re.exec(source)) !== null) {
    names.add(m[1]);
  }
  return names;
}

function extractPushedNames(source: string): string[] {
  const names: string[] = [];
  const re = /router\.(?:push|replace)\(\s*\{[^}]*?name:\s*'([a-z0-9-]+)'/gi;
  let m: RegExpExecArray | null;
  while ((m = re.exec(source)) !== null) {
    names.push(m[1]);
  }
  return names;
}

describe('router name references', () => {
  const adminRoutesSrc = readFileSync(resolve(SRC_DIR, 'modules/admin/routes.ts'), 'utf8');
  const rootRouterSrc = readFileSync(resolve(SRC_DIR, 'router/index.ts'), 'utf8');
  const definedNames = new Set<string>([
    ...extractRouteNames(adminRoutesSrc),
    ...extractRouteNames(rootRouterSrc),
  ]);

  it('parses at least one defined route', () => {
    expect(definedNames.size).toBeGreaterThan(0);
  });

  it('every router.push/replace({ name }) resolves to a defined route', () => {
    const files = walk(SRC_DIR);
    const unresolved: { file: string; name: string }[] = [];

    for (const file of files) {
      const src = readFileSync(file, 'utf8');
      for (const name of extractPushedNames(src)) {
        if (!definedNames.has(name)) {
          unresolved.push({ file: file.replace(SRC_DIR + '/', ''), name });
        }
      }
    }

    expect(unresolved, `Unresolved route names: ${JSON.stringify(unresolved, null, 2)}`).toEqual([]);
  });
});
