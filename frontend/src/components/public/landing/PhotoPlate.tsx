import type { CSSProperties } from "react";

/** Responsive photography with a reserved aspect ratio to prevent layout shifts. */

export type PhotoPlateProps = {
  /** Largest source, plus the 800w variant for narrow viewports. */
  src: string;
  srcSmall: string;
  /** Intrinsic size of `src`. Sources are cropped differently, so this varies. */
  width: number;
  height: number;
  alt: string;
  /** CSS aspect-ratio for the crop. The sources are 4:3; omit to let the
      class govern, which is how the wide plate varies by breakpoint. */
  ratio?: string;
  /** object-position, to keep the subject in frame when cropping. */
  focus?: string;
  /** Above-the-fold plates should not lazy-load. */
  priority?: boolean;
  className?: string;
};

export function PhotoPlate({
  src,
  srcSmall,
  width,
  height,
  alt,
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
    </figure>
  );
}
