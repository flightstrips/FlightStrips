import { useAuth0 } from "@auth0/auth0-react";
import { useLocation } from "react-router";

export default function Authentication() {
  const { loginWithRedirect } = useAuth0();
  const location = useLocation();
  const returnTo = (location.state as { returnTo?: string })?.returnTo ?? "/app";

  return (
    <div className="min-h-screen flex">
      {/* Left panel – primary green branding (hidden on mobile) */}
      <div className="hidden md:flex w-full md:w-[45%] min-h-screen bg-primary flex-col items-center justify-center px-8 py-12">
        <div className="text-center">
          <h1
            className="font-display font-semibold text-3xl md:text-4xl text-white tracking-tight"
            style={{ letterSpacing: "-0.02em" }}
          >
            FlightStrips
          </h1>
          <p className="mt-4 text-white/80 text-sm max-w-xs mx-auto font-light">
            ATC strip management for simulation.
          </p>
        </div>
      </div>

      {/* Right panel – environment selection */}
      <div className="w-full md:w-[55%] min-h-screen bg-cream flex flex-col items-center justify-center px-8 py-12">
        <div className="w-full max-w-md">
          <p className="text-xs tracking-widest uppercase text-navy/60 mb-2">
            Sign in
          </p>
          <h2
            className="font-display font-semibold text-2xl md:text-3xl text-navy mb-8"
            style={{ letterSpacing: "-0.02em" }}
          >
            Choose your environment
          </h2>

          <div className="space-y-4">
            {/* Live – disabled */}
            <button
              disabled
              className="w-full p-5 rounded-lg border border-navy/15 bg-white/50 flex flex-col items-center gap-3 hover:bg-white/70 transition-colors disabled:opacity-60 disabled:cursor-not-allowed disabled:hover:bg-white/50"
            >
              <div className="flex items-center gap-2">
                <div className="w-2 h-2 rounded-full bg-amber-500" />
                <span className="text-xs font-mono text-navy/60 tracking-wider">
                  LIVE · DISABLED
                </span>
              </div>
              <div className="aspect-video w-28 rounded border border-navy/10 bg-navy/5 flex items-center justify-center">
                <span className="text-xs text-navy/50 uppercase tracking-widest font-medium">
                  VATSIM
                </span>
              </div>
              <span className="text-sm font-medium text-navy">Live</span>
            </button>

            {/* Development – active */}
            <button
              onClick={() =>
                loginWithRedirect({
                  authorizationParams: { connection: "vatsim-dev" },
                  appState: { returnTo },
                })
              }
              className="w-full p-5 rounded-lg border border-primary/30 bg-white flex flex-col items-center gap-3 hover:bg-primary/5 hover:border-primary/50 transition-colors"
            >
              <div className="flex items-center gap-2">
                <div className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
                <span className="text-xs font-mono text-navy/60 tracking-wider">
                  DEV · ACTIVE
                </span>
              </div>
              <div className="aspect-video w-28 rounded border border-primary/20 bg-primary/5 flex items-center justify-center">
                <span className="text-xs text-primary uppercase tracking-widest font-medium">
                  VATSIM
                </span>
              </div>
              <span className="text-sm font-medium text-navy">
                Local Development
              </span>
            </button>
          </div>

          <div className="mt-10 pt-6 border-t border-navy/10 text-center">
            <p className="text-xs text-navy/60">
              FlightStrips is free and open-source.{" "}
              <a
                href="https://github.com"
                target="_blank"
                rel="noopener noreferrer"
                className="text-primary hover:underline"
              >
                GitHub
              </a>
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
