import { Link } from "react-router";

export type PublicFooterTone = "landing" | "industrial" | "default";

export function PublicFooter({ tone = "landing" }: { tone?: PublicFooterTone }) {
  if (tone === "industrial" || tone === "default") {
    return (
      <footer className="mt-auto border-t border-cream/10 dark:border-white/10 bg-navy dark:bg-card text-cream dark:text-foreground py-10 px-6 md:px-8">
        <div className="max-w-5xl mx-auto flex flex-col items-center gap-6 text-center">
          <nav className="flex flex-wrap items-center justify-center gap-x-6 gap-y-1 text-[13px] tracking-tight">
            <a href="https://docs.flightstrips.dk" className="text-cream/80 dark:text-foreground/80 hover:text-cream dark:hover:text-foreground transition-colors">
              Docs
            </a>
            <Link to="/privacy" className="text-cream/80 dark:text-foreground/80 hover:text-cream dark:hover:text-foreground transition-colors">
              Privacy
            </Link>
            <Link to="/data-handling" className="text-cream/80 dark:text-foreground/80 hover:text-cream dark:hover:text-foreground transition-colors">
              Data Handling
            </Link>
          </nav>
          <p className="text-xs text-cream/60 dark:text-foreground/60">For simulation use only. Not for real-world operations.</p>
        </div>
      </footer>
    );
  }
  const footerColumns = [
    {
      title: "Product",
      links: [
        { label: "Introduction", href: "https://docs.flightstrips.dk/getting-started/intro/" },
        { label: "Features", href: "https://docs.flightstrips.dk/getting-started/features/" },
        { label: "EuroScope plugin", href: "https://docs.flightstrips.dk/getting-started/es-plugin/" },
        { label: "Alpha tester", href: "https://docs.flightstrips.dk/dev/alpha-tester/" },
      ],
    },
    {
      title: "Concepts",
      links: [
        { label: "Pre-departure clearance", href: "https://docs.flightstrips.dk/concepts/pre-departure-clearance/" },
        { label: "Ownership", href: "https://docs.flightstrips.dk/concepts/ownership/" },
        { label: "Validation status", href: "https://docs.flightstrips.dk/procedures/validation-status/" },
        { label: "Memory aids", href: "https://docs.flightstrips.dk/procedures/memory-aids/" },
        { label: "Alone as Tower", href: "https://docs.flightstrips.dk/procedures/twr-bandbox/" },
      ],
    }
  ];

  return (
    <footer className="bg-[#051415] text-white border-t border-[#233434] py-20 px-[60px]">
      <div className="max-w-full mx-auto">
        {/* Footer Grid */}
        <div className="grid grid-cols-5 gap-10 mb-15 pb-8">
          {/* Brand Column */}
          <div className="col-span-2">
            <Link to="/" className="flex items-center gap-2.5 text-lg font-semibold text-white mb-4 w-fit">
              <svg className="w-6 h-6" viewBox="0 0 28 28" fill="none">
                <rect x="3" y="6" width="22" height="4" rx="1" fill="#a0dae4" />
                <rect x="3" y="12" width="22" height="4" rx="1" fill="#a0dae4" opacity="0.7" />
                <rect x="3" y="18" width="22" height="4" rx="1" fill="#a0dae4" opacity="0.4" />
              </svg>
              FlightStrips
            </Link>
            <p className="text-sm text-[#8a9a9a] leading-relaxed max-w-xs">
              A VATSIM flight strip board for tower and ground operations. Built by controllers, for controllers.
            </p>
          </div>

          {/* Footer Columns */}
          {footerColumns.map((column) => (
            <div key={column.title}>
              <h4 className="text-sm font-semibold text-white mb-5">{column.title}</h4>
              <ul className="space-y-3">
                {column.links.map((link) => (
                  <li key={link.label}>
                    <a
                      href={link.href}
                      className="text-sm text-[#8a9a9a] hover:text-[#a0dae4] transition-colors"
                    >
                      {link.label}
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        {/* Footer Bottom */}
        <div className="flex justify-between items-center pt-8 border-t border-[#233434]">
          <div className="flex items-center gap-5 text-sm text-[#8a9a9a]">
            <span>© 2026 FlightStrips.</span>
            <a href="https://docs.flightstrips.dk" className="hover:text-[#a0dae4] transition-colors">
              Docs
            </a>
            <a href="https://github.com/flightstrips" className="hover:text-[#a0dae4] transition-colors">
              GitHub
            </a>
            <span className="opacity-60">Not affiliated with VATSIM.</span>
          </div>

          <div className="flex gap-4">
            <a
              href="https://github.com/flightstrips"
              aria-label="GitHub"
              className="text-[#8a9a9a] hover:text-[#a0dae4] transition-colors inline-flex items-center justify-center w-6 h-6"
            >
              <svg viewBox="0 0 24 24" className="w-4.5 h-4.5 fill-current">
                <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
              </svg>
            </a>
            <a
              href="https://discord.gg/vpBk2mpg74"
              aria-label="Discord"
              className="text-[#8a9a9a] hover:text-[#a0dae4] transition-colors inline-flex items-center justify-center w-6 h-6"
            >
              <svg viewBox="0 0 24 24" className="w-4.5 h-4.5 fill-current">
                <path d="M20.317 4.3698a19.7913 19.7913 0 00-4.8851-1.5152.0741.0741 0 00-.0785.0371c-.211.3753-.4447.8648-.6083 1.2495-1.8447-.2762-3.68-.2762-5.4868 0-.1636-.3933-.4058-.8742-.6177-1.2495a.077.077 0 00-.0785-.037 19.7363 19.7363 0 00-4.8852 1.515.0699.0699 0 00-.0321.0277C.5334 9.0458-.319 13.5799.0992 18.0578a.0824.0824 0 00.0312.0561c2.0528 1.5076 4.0413 2.4228 5.9929 3.0294a.0777.0777 0 00.0842-.0276c.4616-.6304.8731-1.2952 1.226-1.9942a.076.076 0 00-.0416-.1057c-.6528-.2476-1.2743-.5495-1.8722-.8923a.077.077 0 01-.0076-.1277c.1258-.0943.2517-.1923.3718-.2914a.0743.0743 0 01.0776-.0105c3.9278 1.7933 8.18 1.7933 12.0614 0a.0739.0739 0 01.0785.0095c.1202.099.246.1981.3728.2924a.077.077 0 01-.0066.1276 12.2986 12.2986 0 01-1.873.8914.0766.0766 0 00-.0407.1067c.3604.698.7719 1.3628 1.225 1.9932a.076.076 0 00.0842.0286c1.961-.6067 3.9495-1.5219 6.0023-3.0294a.077.077 0 00.0313-.0552c.5004-5.177-.8382-9.6739-3.5485-13.6604a.061.061 0 00-.0312-.0286zM8.02 15.3312c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9555-2.4189 2.157-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.9555 2.4189-2.1569 2.4189zm7.9748 0c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9554-2.4189 2.1569-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.946 2.4189-2.1568 2.4189Z" />
              </svg>
            </a>
          </div>
        </div>
      </div>
    </footer>
  );
}
