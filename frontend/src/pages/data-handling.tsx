import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { ScrollProgress } from "@/components/public/ScrollProgress";
import { ScrollReveal } from "@/components/public/ScrollReveal";

export default function DataHandling() {
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
              <p className="text-xs tracking-widest uppercase text-nc-muted">Legal</p>
            </div>
          </ScrollReveal>
          
          <ScrollReveal delay={0.1}>
            <h1 
              className="font-display font-normal text-5xl md:text-7xl mb-6 text-white"
              style={{ letterSpacing: '-0.02em' }}
            >
              Data Handling
            </h1>
          </ScrollReveal>
          
          <ScrollReveal delay={0.2}>
            <p className="text-sm text-nc-muted">Last updated: [Date]</p>
          </ScrollReveal>
        </div>
      </section>

      {/* Content Section */}
      <section className="bg-fs-primary/8 border-t border-fs-primary/25 py-20 px-8">
        <div className="max-w-4xl mx-auto">
          <ScrollReveal>
            <div className="space-y-8">
              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Overview
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                  This document outlines how FlightStrips handles, processes, and stores data within our system. 
                  As a simulation platform, we are committed to transparent data practices and user privacy.
                </p>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Types of Data Processed
                </h2>
                <div className="space-y-4">
                  <div>
                    <h3 className="text-lg text-white font-medium mb-2">Flight Strip Data</h3>
                    <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                      Flight information including callsigns, routes, altitudes, and timing data. This data is 
                      processed in real-time for operational purposes and is not stored permanently.
                    </p>
                  </div>
                  <div>
                    <h3 className="text-lg text-white font-medium mb-2">User Session Data</h3>
                    <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                      Authentication tokens, session identifiers, and user preferences. This data is managed 
                      through secure authentication providers.
                    </p>
                  </div>
                  <div>
                    <h3 className="text-lg text-white font-medium mb-2">System Logs</h3>
                    <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                      Technical logs for system monitoring, error tracking, and performance optimization. Logs 
                      are retained for a limited period for operational purposes.
                    </p>
                  </div>
                </div>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Data Processing Principles
                </h2>
                <ul className="list-disc list-inside space-y-2 text-gray-400 font-light">
                  <li>Data minimization: We collect only what is necessary for system operation</li>
                  <li>Purpose limitation: Data is used only for stated operational purposes</li>
                  <li>Storage limitation: Data is retained only as long as necessary</li>
                  <li>Security: All data is protected with appropriate technical measures</li>
                  <li>Transparency: Users are informed about data processing activities</li>
                </ul>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Data Storage and Retention
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed mb-4">
                  FlightStrips operates with the following data retention policies:
                </p>
                <ul className="list-disc list-inside space-y-2 text-gray-400 font-light">
                  <li>Operational flight data: Processed in real-time, not permanently stored</li>
                  <li>User account data: Retained while the account is active</li>
                  <li>System logs: Retained for 30 days for troubleshooting purposes</li>
                  <li>Backup data: Retained according to operational requirements</li>
                </ul>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Data Security Measures
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed mb-4">
                  We implement multiple layers of security to protect data:
                </p>
                <ul className="list-disc list-inside space-y-2 text-gray-400 font-light">
                  <li>Encryption in transit using TLS/SSL protocols</li>
                  <li>Secure authentication through Auth0</li>
                  <li>Regular security audits and updates</li>
                  <li>Access controls and authentication requirements</li>
                  <li>Network security and firewall protection</li>
                </ul>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Third-Party Services
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                  FlightStrips may use third-party services for authentication, hosting, and analytics. These 
                  services are bound by their own privacy policies and data handling practices. We ensure that 
                  any third-party service meets our security and privacy standards.
                </p>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  User Rights and Requests
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed mb-4">
                  Users may request information about their data or request data deletion by contacting us 
                  through our support channels. We will respond to such requests within a reasonable timeframe.
                </p>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Contact
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                  For questions about data handling practices, please contact us through our GitHub repository 
                  or support channels.
                </p>
              </div>
            </div>
          </ScrollReveal>
        </div>
      </section>

      <PublicFooter />
    </div>
  );
}
