import { Link, useLocation } from "react-router";
import { useAuth0 } from "@auth0/auth0-react";

export function PublicNavigation() {
  const location = useLocation();
  const { user, isLoading } = useAuth0();
  const showOpenApp = !isLoading && !!user;

  return (
    <nav
      className="fixed top-0 left-0 right-0 z-40 flex items-center justify-between px-6 sm:px-8 py-4 border-b border-navy/10 bg-cream/95 backdrop-blur-md"
    >
      <Link
        to="/"
        className="font-display font-semibold text-xl tracking-tight text-navy hover:text-primary transition-colors duration-200"
      >
        FlightStrips
      </Link>
      <div className="flex items-center gap-6 sm:gap-8">
        <Link
          to="/"
          className={`text-sm font-medium transition-colors duration-200 relative py-1 group ${
            location.pathname === "/" ? "text-primary" : "text-navy/80 hover:text-navy"
          }`}
        >
          Home
          <span
            className={`absolute bottom-0 left-0 right-0 h-px bg-primary transition-opacity duration-200 ${
              location.pathname === "/" ? "opacity-100" : "opacity-0 group-hover:opacity-100"
            }`}
            style={{ transform: "translateY(3px)" }}
          />
        </Link>
        <Link
          to="/about"
          className={`text-sm font-medium transition-colors duration-200 relative py-1 group ${
            location.pathname === "/about" ? "text-primary" : "text-navy/80 hover:text-navy"
          }`}
        >
          About
          <span
            className={`absolute bottom-0 left-0 right-0 h-px bg-primary transition-opacity duration-200 ${
              location.pathname === "/about" ? "opacity-100" : "opacity-0 group-hover:opacity-100"
            }`}
            style={{ transform: "translateY(3px)" }}
          />
        </Link>
        <Link
          to="/app"
          className="inline-flex items-center gap-2 bg-fs-primary text-white px-7 py-3.5 text-sm font-medium hover:bg-fs-primary/90 transition-colors"
        >
          {showOpenApp ? "Open App" : "Sign In"}
          <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth="1.5" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 8.25L21 12m0 0l-3.75 3.75M21 12H3"/>
          </svg>
        </Link>
      </div>
    </nav>
  );
}
