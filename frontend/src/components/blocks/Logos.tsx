import { DashedLine } from "./DashedLine";

export function Logos() {
  return (
    <section className="py-20 px-6 sm:px-8 bg-cream">
      <div className="max-w-5xl mx-auto">
        <div className="flex items-center gap-4 mb-12">
          <DashedLine className="flex-1 border-navy/20" />
          <span className="text-[11px] font-medium tracking-[0.2em] uppercase text-navy/60 whitespace-nowrap">
            Partners
          </span>
          <DashedLine className="flex-1 border-navy/20" />
        </div>
        <h2
          className="font-display font-semibold text-3xl sm:text-4xl text-navy tracking-tight text-center mb-4"
          style={{ letterSpacing: "-0.02em" }}
        >
          Trusted by virtual ATC communities
        </h2>
        <p className="text-navy/70 text-center text-sm font-light mb-12">
          From next-gen communities to established networks.
        </p>
        <div className="flex flex-wrap justify-center gap-8 items-center">
          <a
            href="https://vatsim.net"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center justify-center min-h-[80px] px-8 py-6 bg-white border border-navy/10 rounded-xl shadow-sm hover:border-primary/20 hover:shadow transition-all"
          >
            <img
              src="/Postive.svg"
              alt="VATSIM"
              className="h-16 object-contain opacity-90"
            />
          </a>
        </div>
      </div>
    </section>
  );
}
