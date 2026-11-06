import { describe, expect, it } from 'vitest';

import { clampPan, MAX_SCALE, NO_ZOOM, zoomAt } from './zoom';

const stage = { width: 1000, height: 800 };
const content = { width: 1000, height: 500 };
const center = { x: 0, y: 0 };

describe('zoomAt', () => {
  it('zooms around the center', () => {
    expect(zoomAt(NO_ZOOM, 2, center, content, stage)).toEqual({ scale: 2, x: 0, y: 0 });
  });

  // Zooming toward the right edge moves the picture left, so that spot stays put.
  it('keeps the point under the cursor in place', () => {
    const point = { x: 200, y: 0 };
    const zoomed = zoomAt(NO_ZOOM, 2, point, content, stage);

    expect(zoomed.x).toBe(-200);
    expect(zoomed.x + point.x * zoomed.scale).toBeCloseTo(point.x);
  });

  it('stays within the limits', () => {
    expect(zoomAt(NO_ZOOM, 0.5, center, content, stage).scale).toBe(1);
    expect(zoomAt({ scale: 7, x: 0, y: 0 }, 3, center, content, stage).scale).toBe(MAX_SCALE);
  });

  it('recenters when zoomed all the way out', () => {
    const zoomed = { scale: 2, x: -300, y: 40 };
    expect(zoomAt(zoomed, 0.5, { x: 100, y: 100 }, content, stage)).toEqual(NO_ZOOM);
  });
});

describe('clampPan', () => {
  it('stops an edge at the edge of the stage', () => {
    expect(clampPan({ scale: 2, x: 5000, y: 0 }, content, stage).x).toBe(500);
    expect(clampPan({ scale: 2, x: -5000, y: 0 }, content, stage).x).toBe(-500);
  });

  it('keeps a side smaller than the stage centered', () => {
    // 500 * 1.5 is still under the stage's 800.
    expect(clampPan({ scale: 1.5, x: 0, y: 300 }, content, stage).y).toBe(0);
  });

  it('allows panning inside the limits', () => {
    expect(clampPan({ scale: 3, x: 100, y: -50 }, content, stage)).toEqual({ scale: 3, x: 100, y: -50 });
  });
});
