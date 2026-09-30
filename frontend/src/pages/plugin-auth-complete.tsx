import { useAuth0 } from "@auth0/auth0-react";
import { PageHero, PublicPage } from "@/components/public/SiteChrome";
import { ActionButton, ActionLink, Reveal } from "@/components/public/landing/primitives";

const BODY = "text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]";

export default function PluginAuthComplete() {
  const { isAuthenticated, loginWithRedirect } = useAuth0();

  return (
    <PublicPage>
      <PageHero
        eyebrow="EuroScope sign-in complete"
        title="You are logged in."
        lead="FlightStrips for EuroScope has received your sign-in. You can return to EuroScope now, or stay here and continue on the website."
      >
        <div className="mt-10 flex flex-wrap gap-3">
          {isAuthenticated ? (
            <ActionLink label="Open web app" to="/app" />
          ) : (
            <ActionButton label="Sign in" onClick={() => loginWithRedirect({ appState: { returnTo: "/app" } })} />
          )}
          <ActionLink label="Back to homepage" to="/" variant="secondary" />
        </div>
      </PageHero>

      <section>
        <div className="fsl-gutter py-16 sm:py-24">
          <div className="grid border-l border-t border-[var(--fsl-line)] md:grid-cols-2">
            <Reveal className="fsl-hoverbar border-b border-r border-[var(--fsl-line)] bg-[var(--fsl-surface)] p-8 sm:p-10">
              <h2 className="fsl-display mb-4 text-[28px] sm:text-[32px]">Back in EuroScope</h2>
              <p className={BODY}>
                You can return to EuroScope now. Sign-in is complete and FlightStrips is ready.
              </p>
            </Reveal>

            <Reveal delay={100} className="fsl-hoverbar border-b border-r border-[var(--fsl-line)] p-8 sm:p-10">
              <h2 className="fsl-display mb-4 text-[28px] sm:text-[32px]">Continue on the website</h2>
              <p className={`mb-4 ${BODY}`}>
                Browse the public site, open the web app, or just leave this tab open for later. Nothing else is
                required here.
              </p>
              <p className="fsl-eyebrow leading-relaxed">For simulation use only. Not for real-world operations.</p>
            </Reveal>
          </div>
        </div>
      </section>
    </PublicPage>
  );
}
