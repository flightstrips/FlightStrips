import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { ScrollProgress } from "@/components/public/ScrollProgress";
import { ScrollReveal } from "@/components/public/ScrollReveal";

export default function About() {
  return (
    <div className="bg-nc-black min-h-screen text-white">
      <ScrollProgress />
      <PublicNavigation />
      
      {/* Hero Section */}
      <section className="bg-nc-black py-28 px-8">
        <div className="max-w-7xl mx-auto">
          <ScrollReveal>
            <div className="flex items-center gap-4 mb-4">
              <div className="w-8 h-px bg-nc-border"></div>
              <p className="text-xs tracking-widest uppercase text-nc-muted">About</p>
            </div>
          </ScrollReveal>
          
          <ScrollReveal delay={0.1}>
            <h1 
              className="font-display font-normal text-5xl md:text-7xl mb-6 text-white"
              style={{ letterSpacing: '-0.02em' }}
            >
              About Us
            </h1>
          </ScrollReveal>
          
          <ScrollReveal delay={0.2}>
            <p className="text-sm text-nc-muted">Home / About Us</p>
          </ScrollReveal>
        </div>
      </section>

      {/* Vision Section */}
      <section className="bg-fs-primary/8 border-t border-fs-primary/20 py-20 px-8">
        <div className="max-w-7xl mx-auto">
          <div className="grid md:grid-cols-2 gap-12 md:gap-16 items-center">
            <ScrollReveal>
              <div className="flex items-start gap-4">
                <div className="w-8 h-32 bg-gradient-to-b from-fs-primary to-transparent"></div>
                <div>
                  <div className="flex items-center gap-4 mb-4">
                    <div className="w-8 h-px bg-nc-border"></div>
                    <p className="text-xs tracking-widest uppercase text-nc-muted">Vision</p>
                  </div>
                  <h2 
                    className="font-display font-normal text-3xl md:text-5xl mb-8 text-white"
                    style={{ letterSpacing: '-0.02em' }}
                  >
                    Our vision for a next generation strip management system
                  </h2>
                </div>
              </div>
            </ScrollReveal>
            
            <ScrollReveal delay={0.1}>
              <div className="space-y-6">
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                  FlightStrips represents a fundamental reimagining of air traffic control strip management, 
                  designed specifically for virtual ATC environments. We combine precision engineering with 
                  intuitive design to deliver a system that feels both powerful and effortless.
                </p>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                  Built for simulation communities, FlightStrips enables controllers to focus on what matters: 
                  safe, efficient air traffic management. Every feature is crafted with the understanding that 
                  clarity and reliability are non-negotiable in high-stakes environments.
                </p>
              </div>
            </ScrollReveal>
          </div>
        </div>
      </section>

      {/* Values Section */}
      <section className="bg-fs-primary/5 py-20 px-8">
        <div className="max-w-7xl mx-auto">
          <ScrollReveal>
            <div className="flex items-center gap-4 mb-4">
              <div className="w-8 h-px bg-nc-border"></div>
              <p className="text-xs tracking-widest uppercase text-nc-muted">Principles</p>
            </div>
          </ScrollReveal>
          
          <ScrollReveal delay={0.1}>
            <h2 
              className="font-display font-normal text-3xl md:text-5xl mb-16 text-white"
              style={{ letterSpacing: '-0.02em' }}
            >
              Built on core principles
            </h2>
          </ScrollReveal>

          <div className="grid md:grid-cols-3 gap-px bg-nc-border">
            <ScrollReveal delay={0.1}>
              <div className="bg-fs-primary/12 p-8 hover:bg-fs-primary/20 transition-colors duration-300 border border-fs-primary/25">
                <p className="text-xs tracking-widest uppercase text-nc-muted mb-6">0.1</p>
                <div className="w-8 h-px bg-fs-primary mb-6"></div>
                <h3 
                  className="font-display font-normal text-xl md:text-2xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Precision
                </h3>
                <p className="text-gray-400 text-sm leading-relaxed font-light">
                  FlightStrips is designed to match the real life counterpart 1:1 in nearly all scenarios. 
                  Every workflow, interaction, and system behavior mirrors authentic air traffic control operations 
                  for true-to-life simulation and training.
                </p>
              </div>
            </ScrollReveal>
            
            <ScrollReveal delay={0.2}>
              <div className="bg-fs-primary/12 p-8 hover:bg-fs-primary/20 transition-colors duration-300 border border-fs-primary/25">
                <p className="text-xs tracking-widest uppercase text-nc-muted mb-6">0.2</p>
                <div className="w-8 h-px bg-fs-primary mb-6"></div>
                <h3 
                  className="font-display font-normal text-xl md:text-2xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Reliability
                </h3>
                <p className="text-gray-400 text-sm leading-relaxed font-light">
                  All systems are connected and talk instantly and securely together. Real-time synchronization 
                  ensures seamless communication between components, maintaining data integrity and operational 
                  continuity across the entire platform.
                </p>
              </div>
            </ScrollReveal>
            
            <ScrollReveal delay={0.3}>
              <div className="bg-fs-primary/12 p-8 hover:bg-fs-primary/20 transition-colors duration-300 border border-fs-primary/25">
                <p className="text-xs tracking-widest uppercase text-nc-muted mb-6">0.3</p>
                <div className="w-8 h-px bg-fs-primary mb-6"></div>
                <h3 
                  className="font-display font-normal text-xl md:text-2xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Clarity
                </h3>
                <p className="text-gray-400 text-sm leading-relaxed font-light">
                  Expanded information and coordination. Critical data is presented clearly and comprehensively, 
                  enabling controllers to make informed decisions with full situational awareness and seamless 
                  coordination between positions.
                </p>
              </div>
            </ScrollReveal>
          </div>
        </div>
      </section>

      {/* Quote Section */}
      <section className="bg-fs-primary/10 border-t border-fs-primary/25 py-20 px-8">
        <div className="max-w-4xl mx-auto">
          <ScrollReveal>
            <blockquote className="border-l-2 border-fs-primary pl-6">
              <p className="text-gray-300 text-lg md:text-xl font-light italic leading-relaxed mb-4">
                "FlightStrips has transformed how our vACC manages operations. 
                The precision and clarity of the system allows controllers to focus entirely on 
                what they do best. Compated to previous systems, FlightStrips is a game changer."
              </p>
              <footer className="mt-4">
                <p className="text-sm text-white font-medium">VATSCA vACC Director</p>
                <p className="text-xs text-nc-muted">Simon Bjerre</p>
              </footer>
            </blockquote>
          </ScrollReveal>
        </div>
      </section>

      {/* Open Source Section */}
      <section className="bg-fs-primary/8 py-20 px-8">
        <div className="max-w-7xl mx-auto">
          <ScrollReveal>
            <div className="flex items-center gap-4 mb-4">
              <div className="w-8 h-px bg-nc-border"></div>
              <p className="text-xs tracking-widest uppercase text-nc-muted">Open Source</p>
            </div>
          </ScrollReveal>
          
          <ScrollReveal delay={0.1}>
            <h2 
              className="font-display font-normal text-3xl md:text-5xl mb-8 text-white"
              style={{ letterSpacing: '-0.02em' }}
            >
              Free and open-source
            </h2>
          </ScrollReveal>
          
          <ScrollReveal delay={0.2}>
            <p className="font-sans font-light text-base text-gray-400 mb-12 max-w-3xl leading-relaxed">
              FlightStrips is a free and open-source project, built by and for the virtual ATC community. 
              Support is available via GitHub, and contributions are welcome from developers and controllers 
              who share our vision for better strip management.
            </p>
          </ScrollReveal>
          
          <ScrollReveal delay={0.3}>
            <div className="flex flex-col sm:flex-row gap-4">
              <a 
                href="https://github.com" 
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-2 bg-fs-primary text-white px-7 py-3.5 text-sm font-medium hover:bg-fs-primary/90 transition-colors"
              >
                View on GitHub
                <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth="1.5" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 8.25L21 12m0 0l-3.75 3.75M21 12H3"/>
                </svg>
              </a>
            </div>
          </ScrollReveal>
        </div>
      </section>

      <PublicFooter />
    </div>
  );
}
