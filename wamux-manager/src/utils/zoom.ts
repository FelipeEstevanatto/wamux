export interface ZoomState {
  scale: number;
  x: number;
  y: number;
}

export interface Size {
  width: number;
  height: number;
}

export const MIN_SCALE = 1;
export const MAX_SCALE = 8;
export const NO_ZOOM: ZoomState = { scale: 1, x: 0, y: 0 };

/**
 * Keeps the picture on screen: an edge never moves past the stage's edge, and a side
 * smaller than the stage stays centered. `content` is the size at scale 1.
 */
export function clampPan(state: ZoomState, content: Size, stage: Size): ZoomState {
  const limitX = Math.max(0, (content.width * state.scale - stage.width) / 2);
  const limitY = Math.max(0, (content.height * state.scale - stage.height) / 2);
  return {
    scale: state.scale,
    // `|| 0` turns -0 into 0.
    x: Math.min(limitX, Math.max(-limitX, state.x)) || 0,
    y: Math.min(limitY, Math.max(-limitY, state.y)) || 0,
  };
}

/**
 * Zooms by `factor`, keeping the point under the cursor where it is. `point` is
 * measured from the center of the stage.
 */
export function zoomAt(
  state: ZoomState,
  factor: number,
  point: { x: number; y: number },
  content: Size,
  stage: Size
): ZoomState {
  const scale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, state.scale * factor));
  const ratio = scale / state.scale;
  return clampPan(
    {
      scale,
      x: point.x - (point.x - state.x) * ratio,
      y: point.y - (point.y - state.y) * ratio,
    },
    content,
    stage
  );
}
