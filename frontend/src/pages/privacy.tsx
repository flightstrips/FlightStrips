import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { ScrollProgress } from "@/components/public/ScrollProgress";
import { ScrollReveal } from "@/components/public/ScrollReveal";

export default function Privacy() {
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
              Privacy Policy
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
                  Introduction
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                  FlightStrips is committed to protecting your privacy. This Privacy Policy explains how we collect, 
                  use, disclose, and safeguard your information when you use our service.
                </p>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Information We Collect
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed mb-4">
                  We collect information that you provide directly to us, including:
                </p>
                <ul className="list-disc list-inside space-y-2 text-gray-400 font-light">
                  <li>Account information (username, email address)</li>
                  <li>Authentication credentials through Auth0</li>
                  <li>Usage data and system interactions</li>
                  <li>Technical information (IP address, browser type, device information)</li>
                </ul>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  How We Use Your Information
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed mb-4">
                  We use the information we collect to:
                </p>
                <ul className="list-disc list-inside space-y-2 text-gray-400 font-light">
                  <li>Provide, maintain, and improve our services</li>
                  <li>Authenticate users and manage accounts</li>
                  <li>Monitor and analyze usage patterns</li>
                  <li>Ensure system security and prevent fraud</li>
                </ul>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Data Sharing and Disclosure
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                  We do not sell, trade, or rent your personal information to third parties. We may share information 
                  only in the following circumstances:
                </p>
                <ul className="list-disc list-inside space-y-2 text-gray-400 font-light mt-4">
                  <li>With your explicit consent</li>
                  <li>To comply with legal obligations</li>
                  <li>To protect our rights and safety</li>
                  <li>With service providers who assist in operations</li>
                </ul>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Data Security
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                  We implement appropriate technical and organizational measures to protect your personal information 
                  against unauthorized access, alteration, disclosure, or destruction. However, no method of transmission 
                  over the internet is 100% secure.
                </p>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Your Rights
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed mb-4">
                  You have the right to:
                </p>
                <ul className="list-disc list-inside space-y-2 text-gray-400 font-light">
                  <li>Access your personal information</li>
                  <li>Correct inaccurate data</li>
                  <li>Request deletion of your data</li>
                  <li>Object to processing of your data</li>
                  <li>Data portability</li>
                </ul>
              </div>

              <div>
                <h2 
                  className="font-display font-normal text-2xl md:text-3xl mb-4 text-white"
                  style={{ letterSpacing: '-0.01em' }}
                >
                  Contact Us
                </h2>
                <p className="font-sans font-light text-base text-gray-400 leading-relaxed">
                  If you have questions about this Privacy Policy, please contact us through our GitHub repository 
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
