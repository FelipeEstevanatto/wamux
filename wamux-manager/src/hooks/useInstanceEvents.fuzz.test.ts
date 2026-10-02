import { describe, expect, it } from 'vitest';
import * as fc from 'fast-check';

import { parseFrame } from './useInstanceEvents';

/** True when `s` parses to a non-null object (the only shape parseFrame keeps). */
function isJsonObject(s: string): boolean {
  try {
    const parsed = JSON.parse(s);
    return parsed !== null && typeof parsed === 'object';
  } catch {
    return false;
  }
}

/**
 * Fuzz the /ws frame parser. The server sends `{ queue, payload }` where
 * `payload` is the event JSON as a string. A malformed frame must be dropped
 * (null), never thrown — a throwing parser would break the message pump.
 *
 * The "unit fuzzing" property style is used (not vitest's native fuzz runner)
 * so this runs offline, deterministically, in CI.
 */
describe('parseFrame (property/fuzz)', () => {
  it('never throws on arbitrary input', () => {
    fc.assert(
      fc.property(fc.string(), (raw) => {
        expect(() => parseFrame(raw)).not.toThrow();
        const result = parseFrame(raw);
        // Either null, or a well-shaped event object.
        if (result !== null) {
          expect(typeof result.queue).toBe('string');
          expect(typeof result.event).toBe('string');
          expect(typeof result.data).toBe('object');
          expect(typeof result.raw).toBe('object');
        }
      }),
      { numRuns: 1000 }
    );
  });

  it('drops a frame whose payload is not a JSON object', () => {
    fc.assert(
      fc.property(fc.string(), (payload) => {
        // A non-object payload (bad JSON, a bare string/number/null) must be
        // rejected: the handler expects an event object.
        if (!isJsonObject(payload)) {
          const raw = JSON.stringify({ queue: 'q', payload });
          expect(parseFrame(raw)).toBeNull();
        }
      }),
      { numRuns: 1000 }
    );
  });

  it('round-trips a well-formed frame', () => {
    fc.assert(
      fc.property(
        fc.string(),
        fc.record({ event: fc.string(), data: fc.anything() }),
        (queue, event) => {
          const raw = JSON.stringify({
            queue,
            payload: JSON.stringify({ event: event.event, data: event.data }),
          });
          const result = parseFrame(raw);
          expect(result).not.toBeNull();
          expect(result?.queue).toBe(queue);
          expect(result?.event).toBe(event.event);
        }
      ),
      { numRuns: 500 }
    );
  });
});