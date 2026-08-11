/*
 * Wait for a condition, not for a duration.
 *
 * `waitForTimeout(300)` then assert is a bet that the machine is not busy. It
 * holds while you run one suite by hand and stops holding when ten of them run
 * after two builds — which is how `shell` and `builder` failed in a sweep and
 * passed on their own. A flaky gate is a gate people learn to re-run.
 *
 * Polling returns as soon as the condition holds, so this is also faster than
 * the fixed sleep it replaces.
 */
const STEP = 40
const LIMIT = 6000

/** Resolves true once read() equals want, or false if it never does. */
export async function until(read, want, limit = LIMIT) {
  const deadline = Date.now() + limit
  for (;;) {
    if ((await read()) === want) return true
    if (Date.now() >= deadline) return false
    await new Promise((r) => setTimeout(r, STEP))
  }
}

/**
 * Waits for something to *stay* false — a negative assertion.
 *
 * Polling cannot help here: there is nothing to wait for, and returning early
 * would only mean not having looked yet. A fixed settle is correct, and short,
 * because what it guards against is synchronous.
 */
export function settle(ms = 250) {
  return new Promise((r) => setTimeout(r, ms))
}
