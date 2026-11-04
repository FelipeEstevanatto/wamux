# Bun + jsdom ≥ 30.1 incompatibility (frontend tests)

## Symptom

Running `bun run test` in `wamux-manager/` fails before any test executes:

```
Error: [vitest-pool]: Failed to start forks worker for test files …
Caused by: TypeError: 'addEventListener' called on an object that is not a valid instance of EventTarget.
  ❯ catchWindowErrors node_modules/vitest/dist/chunks/index.*.js:1253:9
```

The same suite passes under Node 24 (`npx vitest run` → 5/5 tests pass), so this is a Bun-specific environment bug, not a test failure.

## Root cause

- jsdom 30.1 reworked its webidl wrappers to brand-check instances with private fields (`WrapperData`, checked via `in wrapper`).
- jsdom 30.1 builds its window with `vm.createContext(vm.constants.DONT_CONTEXTIFY)`.
- Under Bun 1.4.x, `vm.createContext(DONT_CONTEXTIFY)` returns a **different object** than the context's `globalThis`, so the private-field brand does not survive the worker-realm boundary. vitest's `catchWindowErrors(global)` then calls `global.addEventListener("error", …)` and Bun's brand check rejects it.

## Upstream references

- oven-sh/bun #45198 — "jsdom ≥ 30.1 breaks vitest jsdom environment: 'addEventListener' called on an object that is not a valid instance of EventTarget" (closed via PR #46021)
- oven-sh/bun #45102 — "node:vm: createContext(DONT_CONTEXTIFY) returns a different object than the context's globalThis (breaks jsdom ≥ 30.1)" (confirmed bug)
- oven-sh/bun #43671 — "vitest jsdom environment with jsdom 30.1.0: forks worker fails to start under bun" (confirmed bug)

Node 24 is unaffected (`DONT_CONTEXTIFY` makes `createContext` return the global itself), and jsdom 30.0.x (symbol-keyed wrappers) is unaffected.

## Fix applied

- `wamux-manager/package.json`: pin `"jsdom": "30.0.0"` (exact, no `^`).
- `wamux-manager/vitest.config.ts`: comment documenting the pin; do not bump without checking the linked Bun issues first.
- The CI `manager-tests` job still uses Node 24 + `npm install --ignore-scripts` as a belt-and-braces fallback, since Bun ≥ 1.4.4 has not yet shipped the fix for #45102 at the time of writing.

## Re-evaluating

Before bumping jsdom past 30.0.0, run:

```bash
cd wamux-manager
bun install --frozen-lockfile
bun run test
```

If it still fails with the EventTarget error, keep the pin and check the upstream issues above for a fix release. If it passes, remove the pin (back to `^30.1.1`) and delete this note.