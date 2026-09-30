import { Link } from "react-router";
import { PageHero, PublicPage } from "@/components/public/SiteChrome";

export default function DataHandling() {
  return (
    <PublicPage>
      <PageHero eyebrow="Legal" title="Data Handling">
        <p className="fsl-eyebrow mt-8">Last updated: 14/03/2026</p>
      </PageHero>

      <section>
        <div className="fsl-gutter py-16 sm:py-24">
          <div className="fsl-prose">
            <div>
              <h2>
                Overview
              </h2>
              <p>
                This document outlines how FlightStrips handles, processes, and stores data within our system.
                As a simulation platform, we are committed to transparent data practices and user privacy.
              </p>
            </div>

            <div>
              <h2>
                Types of Data Processed
              </h2>
              <div>
                <div>
                  <h3>Flight Strip Data</h3>
                  <p>
                    Flight information including callsigns, routes, altitudes, and timing data. This data is
                    processed in real-time for operational purposes and is not stored permanently.
                  </p>
                </div>
                <div>
                  <h3>User Session Data</h3>
                  <p>
                    Authentication tokens, session identifiers, and user preferences. This data is managed
                    through secure authentication providers.
                  </p>
                </div>
                <div>
                  <h3>System Logs</h3>
                  <p>
                    Technical logs for system monitoring, error tracking, and performance optimization. Logs
                    are retained for a limited period for operational purposes.
                  </p>
                </div>
              </div>
            </div>

            <div>
              <h2>
                Data Processing Principles
              </h2>
              <ul>
                <li>Data minimization: We collect only what is necessary for system operation</li>
                <li>Purpose limitation: Data is used only for stated operational purposes</li>
                <li>Storage limitation: Data is retained only as long as necessary</li>
                <li>Security: All data is protected with appropriate technical measures</li>
                <li>Transparency: Users are informed about data processing activities</li>
              </ul>
            </div>

            <div>
              <h2>
                Data Storage and Retention
              </h2>
              <p>
                FlightStrips operates with the following data retention policies:
              </p>
              <ul>
                <li>Operational flight data: Processed in real-time, not permanently stored</li>
                <li>User account data: Retained while the account is active</li>
                <li>System logs: Retained for 30 days for troubleshooting purposes</li>
                <li>Backup data: Retained according to operational requirements</li>
              </ul>
            </div>

            <div>
              <h2>
                Data Security Measures
              </h2>
              <p>
                We implement multiple layers of security to protect data:
              </p>
              <ul>
                <li>Encryption in transit using TLS/SSL protocols</li>
                <li>Secure authentication through Auth0</li>
                <li>Regular security audits and updates</li>
                <li>Access controls and authentication requirements</li>
                <li>Network security and firewall protection</li>
              </ul>
            </div>

            <div>
              <h2>
                Third-Party Services
              </h2>
              <p>
                FlightStrips may use third-party services for authentication, hosting, and analytics. These
                services are bound by their own privacy policies and data handling practices. We ensure that
                any third-party service meets our security and privacy standards.
              </p>
            </div>

            <div>
              <h2>
                User Rights and Requests
              </h2>
              <p>
                Users may request information about their data or request data deletion via our{" "}
                <Link to="/contact">
                  Contact
                </Link>{" "}
                page.
              </p>
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
