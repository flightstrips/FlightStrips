import { useAuth0 } from "@auth0/auth0-react";
import { PublicNavigation } from "@/components/public/PublicNavigation";
import { ScrollProgress } from "@/components/public/ScrollProgress";
import { ScrollReveal } from "@/components/public/ScrollReveal";

export default function Authentication() {
  const { loginWithRedirect } = useAuth0();

  return (
    <div className="bg-nc-black min-h-screen text-white">
      <ScrollProgress />
      <PublicNavigation />
      
      <section className="min-h-screen flex items-center justify-center px-8 py-28">
        <div className="max-w-4xl mx-auto w-full">
          <div className="grid md:grid-cols-2 gap-px bg-nc-border">
            {/* Left Panel - Branding */}
            <div className="bg-fs-primary/10 p-12 md:p-16 flex flex-col justify-center">
              <ScrollReveal>
                <div className="flex items-center gap-4 mb-4">
                  <div className="w-8 h-px bg-nc-border"></div>
                  <span className="text-xs tracking-widest uppercase text-nc-muted">FlightStrips</span>
                </div>
              </ScrollReveal>
              
              <ScrollReveal delay={0.1}>
                <h1 
                  className="font-display font-normal text-4xl md:text-6xl mb-6 text-white"
                  style={{ letterSpacing: '-0.02em' }}
                >
                  ATC Strip Management
                </h1>
              </ScrollReveal>
              
              <ScrollReveal delay={0.2}>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed mb-8">
                  Sign in to access your control position and manage flight strips with precision and clarity.
                </p>
              </ScrollReveal>
              
              <ScrollReveal delay={0.3}>
                <div className="w-full h-px bg-nc-border mb-8"></div>
                <p className="text-xs text-nc-muted font-mono">
                  SYSTEM · READY
                </p>
              </ScrollReveal>
            </div>

            {/* Right Panel - Authentication */}
            <div className="bg-nc-card p-12 md:p-16 flex flex-col justify-center">
              <ScrollReveal>
                <div className="flex items-center gap-4 mb-4">
                  <div className="w-8 h-px bg-nc-border"></div>
                  <p className="text-xs tracking-widest uppercase text-nc-muted">Sign In</p>
                </div>
              </ScrollReveal>
              
              <ScrollReveal delay={0.1}>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-8 text-white"
                  style={{ letterSpacing: '-0.02em' }}
                >
                  Choose your environment
                </h2>
              </ScrollReveal>

              <div className="space-y-4">
                <ScrollReveal delay={0.2}>
                  <button
                    disabled={true}
                    className="w-full bg-nc-dark border border-nc-border p-6 flex flex-col items-center gap-4 hover:bg-fs-primary/20 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <div className="flex items-center gap-2">
                      <div className="w-2 h-2 rounded-full bg-yellow-400"></div>
                      <span className="text-xs font-mono text-nc-muted">LIVE · DISABLED</span>
                    </div>
                    <div className="aspect-video w-32 bg-gradient-to-br from-blue-900/30 to-nc-card flex items-center justify-center border border-nc-border">
                      <span className="text-xs text-nc-muted uppercase tracking-widest">VATSIM</span>
                    </div>
                    <span className="text-sm text-white font-medium">Live</span>
                  </button>
                </ScrollReveal>

                <ScrollReveal delay={0.3}>
                  <button
                    onClick={() => loginWithRedirect({ authorizationParams: { connection: "vatsim-dev" } })}
                    className="w-full bg-nc-dark border border-nc-border p-6 flex flex-col items-center gap-4 hover:bg-fs-primary/20 transition-colors"
                  >
                    <div className="flex items-center gap-2">
                      <div className="w-2 h-2 rounded-full bg-green-400 animate-pulse"></div>
                      <span className="text-xs font-mono text-nc-muted">DEV · ACTIVE</span>
                    </div>
                    <div className="aspect-video w-32 bg-gradient-to-br from-blue-900/30 to-nc-card flex items-center justify-center border border-nc-border">
                      <span className="text-xs text-nc-muted uppercase tracking-widest">VATSIM</span>
                    </div>
                    <span className="text-sm text-white font-medium">Local Development</span>
                  </button>
                </ScrollReveal>
              </div>

              <ScrollReveal delay={0.4}>
                <div className="mt-12 pt-8 border-t border-nc-border">
                  <p className="text-xs text-nc-muted text-center mb-2">
                    FlightStrips is a free and open-source project.
                  </p>
                  <p className="text-xs text-nc-muted text-center">
                    Support via <a href="https://github.com" target="_blank" rel="noopener noreferrer" className="text-nc-blue hover:text-white transition-colors">GitHub</a>
                  </p>
                </div>
              </ScrollReveal>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
