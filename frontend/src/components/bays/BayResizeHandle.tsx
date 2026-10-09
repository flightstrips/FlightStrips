import type { useBayResize } from "./useBayResize";

type HandleProps = ReturnType<ReturnType<typeof useBayResize>["handleProps"]>;

/** Invisible strip on the top edge of a bay header; the header must be `relative`. */
export function BayResizeHandle(props: HandleProps) {
  return (
    <div
      {...props}
      title="Drag to resize, double-click to reset"
      className="absolute inset-x-0 top-0 z-30 h-1.5 cursor-row-resize touch-none"
    />
  );
}
