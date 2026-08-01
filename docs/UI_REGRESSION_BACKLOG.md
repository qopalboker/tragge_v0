# UI Regression Backlog

Tracks UI issues that are visible to the human eye but not caught by build,
type-check, or unit tests. Each entry is its own fix branch — do not bundle.

## Known open issues (as of 2026-04-19)

### 1. Raw i18n keys visible across admin surfaces

**Symptom**: Users see untranslated keys rendered in-place, e.g. `admin.foo`,
`filters.bar`, `countdown.baz`, instead of their translated strings.

**Known locations** (non-exhaustive — more discovery needed):
- Admin dashboard (`/admin/*`) — keys with `admin.` prefix
- Filter controls (various pages) — keys with `filters.` prefix
- Countdown component — keys with `countdown.` prefix
- Tournament cards (module TBD)

**Hypothesis**: During frontend consolidation (commit `a321752`), i18n
bundles from the three original SPAs were either not merged or were merged
with gaps. Components reference keys that `$t()` cannot resolve, so Vue i18n
falls back to emitting the raw key.

**Next step**: Enumerate all missing keys by scanning the rendered DOM of
each page for strings matching `^[a-z]+\.[a-z]+` that appear where copy
should be. Cross-reference with `apps/user-frontend/src/i18n/locales/en.ts`
and `apps/admin-frontend/src/i18n/locales/en.ts` (plus their `fa.ts`
counterparts). Legacy single-panel locale trees have been removed.

**Branch when fixing**: `fix/restore-i18n-keys-admin` (or narrower per scope).

### 2. Card alignment in tournaments list

**Symptom**: Visual misalignment of tournament cards in `/user/tournaments`.
Exact nature TBD (wrapping? gap inconsistency? mixed card heights?).

**Next step**: Open the page in DevTools, inspect the grid/flex container and
child card computed styles. Look for:
- Missing or mismatched `grid-template-columns` / `flex` rules
- Tokens that resolve to unexpected values (e.g. `--spacing-*` with wrong
  units after the token restoration in `69c1b72`)
- A component that was relying on a legacy class (`.card`, `.grid`) whose
  CSS definition lived in one of the deleted `styles/main.css` files

**Branch when fixing**: `fix/tournaments-card-alignment`.

### 3. _(pending human report)_

<!--
Reserve this slot. The human will enumerate additional UI issues after
further browser testing. Each gets its own branch.
-->

## Process

Before declaring any of the above fixed:
1. Open the browser
2. Verify the specific page listed above
3. Screenshot or describe the before/after
4. Link this backlog entry in the PR; mark it resolved once merged

See `CLAUDE.md` → "Visual regression caveat".
