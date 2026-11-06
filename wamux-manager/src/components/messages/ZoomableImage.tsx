import { useEffect, useRef, useState, type PointerEvent } from 'react';

import { clampPan, NO_ZOOM, zoomAt, type Size, type ZoomState } from '@/utils/zoom';

const WHEEL_STEP = 1.2;
const DOUBLE_CLICK_SCALE = 2.5;

const sizeOf = (element: HTMLElement | null): Size => ({
  width: element?.clientWidth ?? 0,
  height: element?.clientHeight ?? 0,
});

/** The size the image is drawn at before zooming, fitted to the stage. */
function fittedSize(image: HTMLImageElement | null, stage: Size, rotated: boolean): Size {
  const natural = {
    width: (rotated ? image?.naturalHeight : image?.naturalWidth) || 1,
    height: (rotated ? image?.naturalWidth : image?.naturalHeight) || 1,
  };
  const fit = Math.min(stage.width / natural.width, stage.height / natural.height);
  return { width: natural.width * fit, height: natural.height * fit };
}

/**
 * An image filling its stage, with zoom on the wheel, pinch and double click, and
 * panning by dragging. The parent keeps the state, so its toolbar can drive it too.
 */
function ZoomableImage({
  src,
  alt,
  zoom,
  rotation,
  onZoom,
  onLoad,
}: {
  src: string;
  alt: string;
  zoom: ZoomState;
  rotation: number;
  onZoom: (update: (current: ZoomState) => ZoomState) => void;
  onLoad: (size: Size) => void;
}) {
  const stageRef = useRef<HTMLDivElement>(null);
  const imageRef = useRef<HTMLImageElement>(null);
  const pointers = useRef(new Map<number, { x: number; y: number }>());
  const pinch = useRef(0);
  const rotated = rotation % 180 !== 0;
  const [stageSize, setStageSize] = useState<Size>({ width: 0, height: 0 });

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(() => setStageSize(sizeOf(stage)));
    observer.observe(stage);
    return () => observer.disconnect();
  }, []);

  const measure = () => {
    const stage = sizeOf(stageRef.current);
    return { stage, content: fittedSize(imageRef.current, stage, rotated) };
  };

  const fromCenter = (clientX: number, clientY: number) => {
    const box = stageRef.current?.getBoundingClientRect();
    return {
      x: clientX - (box?.left ?? 0) - (box?.width ?? 0) / 2,
      y: clientY - (box?.top ?? 0) - (box?.height ?? 0) / 2,
    };
  };

  // React's wheel listener is passive, and the page must not scroll behind the zoom.
  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      const factor = event.deltaY < 0 ? WHEEL_STEP : 1 / WHEEL_STEP;
      const size = {
        stage: sizeOf(stage),
        content: fittedSize(imageRef.current, sizeOf(stage), rotated),
      };
      const box = stage.getBoundingClientRect();
      const point = {
        x: event.clientX - box.left - box.width / 2,
        y: event.clientY - box.top - box.height / 2,
      };
      onZoom((current) => zoomAt(current, factor, point, size.content, size.stage));
    };
    stage.addEventListener('wheel', onWheel, { passive: false });
    return () => stage.removeEventListener('wheel', onWheel);
  }, [onZoom, rotated]);

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    event.currentTarget.setPointerCapture?.(event.pointerId);
    pointers.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
    pinch.current = 0;
  };

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const last = pointers.current.get(event.pointerId);
    if (!last) return;
    pointers.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
    const { stage, content } = measure();

    if (pointers.current.size === 2) {
      const [a, b] = [...pointers.current.values()];
      const distance = Math.hypot(a.x - b.x, a.y - b.y);
      if (pinch.current > 0) {
        const factor = distance / pinch.current;
        const point = fromCenter((a.x + b.x) / 2, (a.y + b.y) / 2);
        onZoom((current) => zoomAt(current, factor, point, content, stage));
      }
      pinch.current = distance;
      return;
    }

    const dx = event.clientX - last.x;
    const dy = event.clientY - last.y;
    onZoom((current) =>
      clampPan({ ...current, x: current.x + dx, y: current.y + dy }, content, stage)
    );
  };

  const onPointerUp = (event: PointerEvent<HTMLDivElement>) => {
    pointers.current.delete(event.pointerId);
    pinch.current = 0;
  };

  const onDoubleClick = (event: { clientX: number; clientY: number }) => {
    const { stage, content } = measure();
    const point = fromCenter(event.clientX, event.clientY);
    onZoom((current) =>
      current.scale > 1 ? NO_ZOOM : zoomAt(current, DOUBLE_CLICK_SCALE, point, content, stage)
    );
  };

  // A sideways image swaps its box, so it is sized to fit once turned.
  const fit = rotated
    ? { width: stageSize.height, height: stageSize.width, flexShrink: 0 }
    : undefined;

  return (
    <div
      ref={stageRef}
      data-testid="zoom-stage"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onDoubleClick={onDoubleClick}
      className={`flex h-full w-full touch-none items-center justify-center overflow-hidden select-none ${
        zoom.scale > 1 ? 'cursor-grab active:cursor-grabbing' : 'cursor-zoom-in'
      }`}
    >
      <img
        ref={imageRef}
        src={src}
        alt={alt}
        draggable={false}
        onLoad={(event) =>
          onLoad({
            width: event.currentTarget.naturalWidth,
            height: event.currentTarget.naturalHeight,
          })
        }
        style={{
          ...fit,
          transform: `translate(${zoom.x}px, ${zoom.y}px) scale(${zoom.scale}) rotate(${rotation}deg)`,
        }}
        className="h-full w-full object-contain"
      />
    </div>
  );
}

export default ZoomableImage;
