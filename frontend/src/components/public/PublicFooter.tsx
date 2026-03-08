import { Link } from "react-router";

export function PublicFooter() {
  return (
    <footer className="bg-navy text-white">

      <div className="py-16 px-6 sm:px-8">
        <div className="max-w-7xl mx-auto">
          <div className="grid md:grid-cols-3 gap-12 mb-12">
          <div>
            <h3 className="font-display font-semibold text-xl tracking-tight text-white mb-4">FlightStrips</h3>
            <p className="text-sm text-white/80 font-light leading-relaxed">
              Next-generation strip management for ATC simulation. DCL, pushback, holding points, internal comms—on any device.
            </p>
            <p className="text-[11px] text-white/60 mt-4 tracking-wide">(Simulation only)</p>
          </div>
          <div>
            <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-white/60 mb-6">About</p>
            <ul className="space-y-3">
              <li>
                <Link to="/about" className="text-sm text-white/80 hover:text-white transition-colors duration-200">
                  Us
                </Link>
              </li>
              <li>
                <Link to="/faq" className="text-sm text-white/80 hover:text-white transition-colors duration-200">
                  FAQ
                </Link>
              </li>
            </ul>
          </div>
          <div>
            <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-white/60 mb-6">Legal</p>
            <ul className="space-y-3">
              <li>
                <Link to="/privacy" className="text-sm text-white/80 hover:text-white transition-colors duration-200">
                  Privacy Policy
                </Link>
              </li>
              <li>
                <Link to="/data-handling" className="text-sm text-white/80 hover:text-white transition-colors duration-200">
                  Data Handling
                </Link>
              </li>
            </ul>
          </div>
        </div>
        <div className="border-t border-white/10 pt-8 flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
            <p className="text-xs text-white/60">© FlightStrips. All rights reserved.</p>
          </div>
        </div>
      </div>
    </footer>
  );
}
