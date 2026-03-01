import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { ScrollProgress } from "@/components/public/ScrollProgress";
import { ScrollReveal } from "@/components/public/ScrollReveal";
import { Link } from "react-router";

export default function Home() {
  return (
    <div className="bg-nc-black min-h-screen text-white">
      <ScrollProgress />
      <PublicNavigation />
      
      {/* Hero Section */}
      <section 
        className="relative min-h-screen flex items-center justify-center px-8 py-28"
        style={{
          background: `
            radial-gradient(ellipse at 70% 40%, rgba(0,61,72,0.4) 0%, transparent 60%),
            radial-gradient(ellipse at 30% 80%, rgba(0,61,72,0.15) 0%, transparent 50%),
            linear-gradient(to bottom, rgba(0,61,72,0.1) 0%, rgba(10,10,10,0.6) 60%, rgba(10,10,10,1) 100%)
          `
        }}
      >
        <div className="max-w-7xl mx-auto w-full">
          <ScrollReveal>
            <div className="flex items-center gap-4 mb-4">
              <div className="w-8 h-px bg-nc-border"></div>
              <span className="inline-block px-4 py-2 text-xs tracking-widest uppercase text-gray-300 rounded-full"
                    style={{ background: 'rgba(255,255,255,0.07)', border: '1px solid rgba(255,255,255,0.12)', backdropFilter: 'blur(8px)' }}>
                ✦ Next-Gen ATC Management
              </span>
            </div>
          </ScrollReveal>
          
          <ScrollReveal delay={0.1}>
            <h1 
              className="font-display font-normal text-5xl md:text-7xl mb-6 text-white"
              style={{ letterSpacing: '-0.02em' }}
            >
              FlightStrips
            </h1>
          </ScrollReveal>
          
          <ScrollReveal delay={0.2}>
            <p className="font-sans font-light text-xl md:text-2xl text-gray-400 mb-12 max-w-3xl leading-relaxed">
              Experience next-generation strip management with precision, clarity, and enterprise-grade reliability.
            </p>
          </ScrollReveal>
          
          <ScrollReveal delay={0.3}>
            <div className="flex flex-col sm:flex-row gap-4">
              <Link 
                to="/login" 
                className="inline-flex items-center gap-2 bg-fs-primary text-white px-7 py-3.5 text-sm font-medium hover:bg-fs-primary/90 transition-colors"
              >
                Get Started
                <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth="1.5" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 8.25L21 12m0 0l-3.75 3.75M21 12H3"/>
                </svg>
              </Link>
              <Link 
                to="/about" 
                className="inline-flex items-center gap-2 border border-white/20 text-white px-7 py-3.5 text-sm font-medium hover:border-white/50 transition-colors"
              >
                Learn More
              </Link>
            </div>
          </ScrollReveal>
        </div>
      </section>

      {/* Features Section */}
      <section className="bg-fs-primary/5 border-t border-nc-border py-20 px-8">
        <div className="max-w-7xl mx-auto">
          <ScrollReveal>
            <div className="flex items-center gap-4 mb-4">
              <div className="w-8 h-px bg-nc-border"></div>
              <p className="text-xs tracking-widest uppercase text-nc-muted">Features</p>
            </div>
          </ScrollReveal>
          
          <ScrollReveal delay={0.1}>
            <h2 
              className="font-display font-normal text-3xl md:text-5xl mb-16 text-white"
              style={{ letterSpacing: '-0.02em' }}
            >
              Enterprise-grade capabilities
            </h2>
          </ScrollReveal>

          <div className="grid md:grid-cols-3 gap-px bg-nc-border">
            <ScrollReveal delay={0.1}>
              <div className="bg-fs-primary/10 p-8 transition-colors duration-300 hover:bg-fs-primary/25 border border-fs-primary/20">
                <p className="text-xs tracking-widest uppercase text-nc-muted mb-6">PDC</p>
                <div className="w-8 h-px bg-fs-primary mb-6"></div>
                <h3 
                  className="font-display font-normal text-xl md:text-2xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Pre-Departure Clearance
                </h3>
                <p className="text-gray-400 text-sm leading-relaxed font-light">
                  Streamlined clearance delivery with automated routing and conflict detection.
                </p>
              </div>
            </ScrollReveal>
            
            <ScrollReveal delay={0.2}>
              <div className="bg-fs-primary/10 p-8 transition-colors duration-300 hover:bg-fs-primary/25 border border-fs-primary/20">
                <p className="text-xs tracking-widest uppercase text-nc-muted mb-6">BARS</p>
                <div className="w-8 h-px bg-fs-primary mb-6"></div>
                <h3 
                  className="font-display font-normal text-xl md:text-2xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Bay Assignment & Routing
                </h3>
                <p className="text-gray-400 text-sm leading-relaxed font-light">
                  Intelligent gate and stand assignment with optimized taxi routing.
                </p>
              </div>
            </ScrollReveal>
            
            <ScrollReveal delay={0.3}>
              <div className="bg-fs-primary/10 p-8 transition-colors duration-300 hover:bg-fs-primary/25 border border-fs-primary/20">
                <p className="text-xs tracking-widest uppercase text-nc-muted mb-6">vACDM</p>
                <div className="w-8 h-px bg-fs-primary mb-6"></div>
                <h3 
                  className="font-display font-normal text-xl md:text-2xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Virtual A-CDM
                </h3>
                <p className="text-gray-400 text-sm leading-relaxed font-light">
                  Collaborative decision-making with real-time data synchronization.
                </p>
              </div>
            </ScrollReveal>
          </div>
        </div>
      </section>

      {/* Partner Section */}
      <section className="bg-fs-primary/8 py-20 px-8">
        <div className="max-w-7xl mx-auto">
          <ScrollReveal>
            <div className="flex items-center gap-4 mb-4">
              <div className="w-8 h-px bg-nc-border"></div>
              <p className="text-xs tracking-widest uppercase text-nc-muted">Partners</p>
            </div>
          </ScrollReveal>
          
          <ScrollReveal delay={0.1}>
            <h2 
              className="font-display font-normal text-3xl md:text-5xl mb-16 text-white"
              style={{ letterSpacing: '-0.02em' }}
            >
              Trusted by virtual ATC communities
            </h2>
          </ScrollReveal>

          <ScrollReveal delay={0.2}>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-8 items-center">
              <div className="aspect-video bg-gradient-to-br from-blue-900/30 to-nc-card flex items-center justify-center border border-nc-border">
                <span className="text-xs text-nc-muted uppercase tracking-widest">
                    <img src="/Negative.svg" alt="VATSIM" className="h-24" />
                </span>
              </div>
            </div>
          </ScrollReveal>
        </div>
      </section>

      {/* CTA Section */}
      <section className="bg-fs-primary/15 border-t border-fs-primary/30 py-28 px-8">
        <div className="max-w-7xl mx-auto">
          <ScrollReveal>
            <div className="flex items-center gap-4 mb-4">
              <div className="w-8 h-px bg-nc-border"></div>
              <p className="text-xs tracking-widest uppercase text-nc-muted">Get Started</p>
            </div>
          </ScrollReveal>
          
          <ScrollReveal delay={0.1}>
            <h2 
              className="font-display font-normal text-3xl md:text-5xl mb-8 text-white"
              style={{ letterSpacing: '-0.02em' }}
            >
              Ready to transform your ATC operations?
            </h2>
          </ScrollReveal>
          
          <ScrollReveal delay={0.2}>
            <p className="font-sans font-light text-lg text-gray-400 mb-12 max-w-2xl leading-relaxed">
              Join virtual air traffic control communities worldwide using FlightStrips for simulation and training.
            </p>
          </ScrollReveal>
          
          <ScrollReveal delay={0.3}>
            <Link 
              to="/login" 
              className="inline-flex items-center gap-2 bg-fs-primary text-white px-7 py-3.5 text-sm font-medium hover:bg-fs-primary/90 transition-colors"
            >
              Sign In
              <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth="1.5" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 8.25L21 12m0 0l-3.75 3.75M21 12H3"/>
              </svg>
            </Link>
          </ScrollReveal>
        </div>
      </section>

      <PublicFooter />
    </div>
  );
}
