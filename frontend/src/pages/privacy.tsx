import { Link } from "react-router";
import { PageHero, PublicPage } from "@/components/public/SiteChrome";

export default function Privacy() {
  return (
    <PublicPage>
      <PageHero eyebrow="Legal" title="Privacy Policy">
        <p className="fsl-eyebrow mt-8">Last updated: 14/03/2026</p>
      </PageHero>

      <section>
        <div className="fsl-gutter py-16 sm:py-24">
          <div className="fsl-prose">
            <div>
              <h2>
                Introduction
              </h2>
              <p>
                FlightStrips is committed to protecting your privacy. This Privacy Policy explains how we
                collect, use, disclose, and safeguard your information when you use our service.
              </p>
            </div>

            <div>
              <h2>
                Information We Collect
              </h2>
              <p>
                We collect information that you provide directly to us, including:
              </p>
              <ul>
                <li>
                  Account information (Name, Vatsim ID, Vatsim Region/Division/Subdivision and Vatsim
                  Rating)
                </li>
                <li>Authentication credentials through Auth0</li>
                <li>Usage data and system interactions</li>
                <li>Technical information (IP address, browser type, device information)</li>
              </ul>
            </div>

            <div>
              <h2>
                How We Use Your Information
              </h2>
              <p>
                We use the information we collect to:
              </p>
              <ul>
                <li>Provide, maintain, and improve our services</li>
                <li>Authenticate users and manage accounts</li>
                <li>Monitor and analyze usage patterns</li>
                <li>Ensure system security and prevent fraud</li>
              </ul>
            </div>

            <div>
              <h2>
                Data Sharing and Disclosure
              </h2>
              <p>
                We do not sell, trade, or rent your personal information to third parties. We may share
                information only in the following circumstances:
              </p>
              <ul>
                <li>With your explicit consent</li>
                <li>To comply with legal obligations</li>
                <li>To protect our rights and safety</li>
                <li>With service providers who assist in operations</li>
              </ul>
            </div>

            <div>
              <h2>
                Data Security
              </h2>
              <p>
                We implement appropriate technical and organizational measures to protect your personal
                information against unauthorized access, alteration, disclosure, or destruction. However, no
                method of transmission over the internet is 100% secure.
              </p>
            </div>

            <div>
              <h2>
                Your Rights
              </h2>
              <p>
                You have the right to:
              </p>
              <ul>
                <li>Access your personal information</li>
                <li>Correct inaccurate data</li>
                <li>Request deletion of your data</li>
                <li>Object to processing of your data</li>
                <li>Data portability</li>
              </ul>
            </div>

            <p>
              For any enquiries, please use our{" "}
              <Link to="/contact">
                Contact
              </Link>{" "}
              page.
            </p>
          </div>
        </div>
      </section>
    </PublicPage>
  );
}
