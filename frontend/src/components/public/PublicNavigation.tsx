import { Link, useLocation } from "react-router";

export function PublicNavigation() {
  const location = useLocation();

  return (
    <nav 
      className="fixed top-0 left-0 right-0 z-40 flex items-center justify-between px-8 py-5"
      style={{ background: 'linear-gradient(to bottom, rgba(0,61,72,0.15) 0%, rgba(10,10,10,0.95) 50%, transparent)' }}
    >
      <Link to="/" className="text-white font-display font-normal text-xl">
        FlightStrips
      </Link>
      <div className="flex items-center gap-8">
        <Link 
          to="/" 
          className={`text-sm transition-colors relative group ${
            location.pathname === '/' 
              ? 'text-white' 
              : 'text-gray-400 hover:text-white'
          }`}
        >
          Home
          <span 
            className={`absolute bottom-0 left-0 right-0 h-px bg-fs-primary transition-opacity ${
              location.pathname === '/' 
                ? 'opacity-100' 
                : 'opacity-0 group-hover:opacity-100'
            }`}
            style={{ transform: 'translateY(4px)' }}
          />
        </Link>
        <Link 
          to="/about" 
          className={`text-sm transition-colors relative group ${
            location.pathname === '/about' 
              ? 'text-white' 
              : 'text-gray-400 hover:text-white'
          }`}
        >
          About
          <span 
            className={`absolute bottom-0 left-0 right-0 h-px bg-fs-primary transition-opacity ${
              location.pathname === '/about' 
                ? 'opacity-100' 
                : 'opacity-0 group-hover:opacity-100'
            }`}
            style={{ transform: 'translateY(4px)' }}
          />
        </Link>
        <Link 
          to="/login" 
          className="inline-flex items-center gap-2 bg-fs-primary text-white px-7 py-3.5 text-sm font-medium hover:bg-fs-primary/90 transition-colors"
        >
          Sign In
          <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth="1.5" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 8.25L21 12m0 0l-3.75 3.75M21 12H3"/>
          </svg>
        </Link>
      </div>
    </nav>
  );
}
