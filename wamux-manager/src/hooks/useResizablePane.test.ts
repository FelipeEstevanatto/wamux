import { describe, expect, it } from 'vitest';
import { clampPaneWidth, PANE_MAX, PANE_MIN } from './useResizablePane';

describe('clampPaneWidth', () => {
  it('keeps a width inside the limits', () => {
    expect(clampPaneWidth(400, 1600)).toBe(400);
    expect(clampPaneWidth(10, 1600)).toBe(PANE_MIN);
    expect(clampPaneWidth(5000, 1600)).toBe(PANE_MAX);
  });

  it('leaves room for the neighbouring pane', () => {
    expect(clampPaneWidth(600, 800)).toBe(440);
  });

  it('never goes below the minimum in a narrow container', () => {
    expect(clampPaneWidth(600, 400)).toBe(PANE_MIN);
  });
});
