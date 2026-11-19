/** The real board, with its complete layout and operational strip colours. */
export function HeroPreview() {
  return (
    <figure className="fsl-hero-preview">
      <a
        href="/fsdemo.png"
        target="_blank"
        rel="noopener noreferrer"
        aria-label="View the FlightStrips board at full size (opens in a new tab)"
        className="fsl-hero-preview__image"
      >
        <img
          src="/fsdemo.png"
          alt="FlightStrips showing Kastrup arrivals and departures across taxiway, startup, pushback and clearance bays."
          width={1918}
          height={1030}
          fetchPriority="high"
          decoding="async"
        />
      </a>
      <figcaption>
        <span>The FlightStrips board at EKCH</span>
        <a href="/fsdemo.png" target="_blank" rel="noopener noreferrer">View full size<span className="sr-only"> (opens in a new tab)</span></a>
      </figcaption>
    </figure>
  );
}
