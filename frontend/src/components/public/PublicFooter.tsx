import { Link } from "react-router";

export function PublicFooter() {
  return (
    <footer className="border-t border-fs-primary/30 bg-fs-primary/8 py-16 px-8">
      <div className="max-w-7xl mx-auto">
        <div className="grid md:grid-cols-3 gap-12 mb-12">
          <div>
            <h3 className="font-display font-normal text-xl mb-4 text-white">FlightStrips</h3>
            <p className="text-sm text-gray-400 font-light leading-relaxed">
              Next-generation strip management system for air traffic control simulation.
            </p>
            <p className="text-xs text-nc-muted mt-4">(Only for simulation)</p>
          </div>
          <div>
            <p className="text-xs tracking-widest uppercase text-nc-muted mb-6">About</p>
            <ul className="space-y-3">
              <li>
                <Link to="/about" className="text-sm text-gray-400 hover:text-white transition-colors">
                  Us
                </Link>
              </li>
            </ul>
          </div>
          <div>
            <p className="text-xs tracking-widest uppercase text-nc-muted mb-6">Legal</p>
            <ul className="space-y-3">
              <li>
                <Link to="/privacy" className="text-sm text-gray-400 hover:text-white transition-colors">
                  Privacy Policy
                </Link>
              </li>
              <li>
                <Link to="/data-handling" className="text-sm text-gray-400 hover:text-white transition-colors">
                  Data Handling
                </Link>
              </li>
            </ul>
          </div>
        </div>
        <div className="border-t border-nc-border pt-8 flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
          <p className="text-xs text-nc-muted">© FlightStrips. All rights reserved.</p>
        </div>
      </div>
    </footer>
  );
}
