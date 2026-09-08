import type { CSSProperties } from "react";

/**
 * A photograph presented as an instrument plate: hairline frame, mono corner
 * annotations, an index number on a rule, and a registration mark. The
 * treatment comes from the art direction reference for these images, and it
 * reuses the page's own mono/hairline vocabulary so photography and diagrams
 * read as one system.
 */

export type PhotoPlateProps = {
  /** Largest source, plus the 800w variant for narrow viewports. */
  src: string;
  srcSmall: string;
  /** Intrinsic size of `src`. Sources are cropped differently, so this varies. */
  width: number;
  height: number;
  alt: string;
  /** Two-digit plate number, shown bottom-left. */
  index: string;
  /** Top-left annotation, one word per line. */
  topLeft: readonly string[];
  /** Bottom-right annotation, one word per line. */
  bottomRight: readonly string[];
  /** CSS aspect-ratio for the crop. The sources are 4:3; omit to let the
      class govern, which is how the wide plate varies by breakpoint. */
  ratio?: string;
  /** object-position, to keep the subject in frame when cropping. */
  focus?: string;
  /** Above-the-fold plates should not lazy-load. */
  priority?: boolean;
  className?: string;
};

function Annotation({
  lines,
  align,
  className,
}: {
  lines: readonly string[];
  align: "left" | "right";
  className: string;
}) {
  return (
    <div className={`pointer-events-none absolute z-10 ${align === "right" ? "text-right" : ""} ${className}`}>
      {lines.map((line) => (
        <span key={line} className="fsl-plate__label">
          {line}
        </span>
      ))}
      <span aria-hidden="true" className={`fsl-plate__rule fsl-plate__rule--${align}`} />
    </div>
  );
}

export function PhotoPlate({
  src,
  srcSmall,
  width,
  height,
  alt,
  index,
  topLeft,
  bottomRight,
  ratio,
  focus = "center",
  priority = false,
  className = "",
}: PhotoPlateProps) {
  return (
    <figure
      className={`fsl-plate relative m-0 overflow-hidden ${className}`}
      style={ratio ? ({ ["--plate-ratio" as string]: ratio } as CSSProperties) : undefined}
    >
      <img
        src={src}
        srcSet={`${srcSmall} 800w, ${src} ${width}w`}
        sizes="(max-width: 820px) 100vw, 1344px"
        alt={alt}
        width={width}
        height={height}
        loading={priority ? "eager" : "lazy"}
        decoding="async"
        className="absolute inset-0 h-full w-full object-cover"
        style={{ objectPosition: focus }}
      />

      {/* Vignette so the annotations stay legible over any part of the frame. */}
      <span aria-hidden="true" className="fsl-plate__grade" />

      <Annotation lines={topLeft} align="left" className="left-5 top-5 sm:left-7 sm:top-7" />
      <Annotation
        lines={bottomRight}
        align="right"
        // Always sits above the index rule rather than opposite the top-left
        // annotation, so the two never collide on a narrow plate.
        className="bottom-16 right-5 sm:right-7"
      />

      <figcaption className="pointer-events-none absolute bottom-5 left-5 z-10 flex items-center gap-4 sm:bottom-7 sm:left-7">
        <span className="fsl-plate__index">{index}</span>
        <span aria-hidden="true" className="fsl-plate__rule fsl-plate__rule--inline" />
      </figcaption>

      <span aria-hidden="true" className="fsl-plate__mark">
        +
      </span>
    </figure>
  );
}
