import { useEffect } from "react";
import "./landing.css";

import { CONNECTIONS, HERO, OWNERSHIP, PHOTOS, SCOPE, SITE } from "./content";
import { CapabilityStory } from "./CapabilityStory";
import { HeroPreview } from "./HeroPreview";
import { LandingFooter } from "./LandingFooter";
import { LandingHeader } from "./LandingHeader";
import { PhotoPlate } from "./PhotoPlate";
import { PositionExplorer } from "./PositionExplorer";
import { ActionButton, ActionLink, Eyebrow, Section, TextLink } from "./primitives";
import { SystemDiagram } from "./SystemDiagram";
import { useLandingAuth } from "./useLandingAuth";

function PrimaryAction({ label = "Open the board" }: { label?: string }) {
  const { isAuthenticated, isSigningIn, signInError, signIn } = useLandingAuth();

  if (isAuthenticated) {
    return <ActionLink label={label} to="/app" />;
  }

  return (
    <div className="fsl-sign-in">
      <ActionButton label="Sign in with VATSIM" onClick={signIn} pending={isSigningIn} />
      {signInError ? <p role="alert" className="fsl-auth-error">{signInError}</p> : null}
    </div>
  );
}

export function LandingPage() {
  // Arriving from another page with a section hash: the section did not exist
  // when the browser tried to scroll to it, so do it once the page is mounted.
  useEffect(() => {
    const id = window.location.hash.slice(1);
    if (id) document.getElementById(id)?.scrollIntoView();
  }, []);

  return (
    <div className="fsl min-h-screen">
      <LandingHeader />

      <main>
        {/* ── Hero ──────────────────────────────────────────── */}
        <section data-fsl-theme="dark" className="fsl-hero">
          <div className="fsl-gutter">
            <div className="fsl-hero__layout">
              <div>
                <p className="fsl-eyebrow mb-6">{HERO.badge}</p>
                <h1 className="fsl-hero__title">{HERO.headline}</h1>
                <p className="fsl-hero__lead">{HERO.standfirst}</p>
                <div className="flex flex-wrap items-start gap-3">
                  <PrimaryAction />
                  <ActionLink label="Set up FlightStrips" href="#setup" variant="secondary" />
                </div>
                <p className="mt-6 text-sm text-[var(--fsl-ink-muted)]">{HERO.note}</p>
              </div>
              <HeroPreview />
            </div>

            <div id="setup" className="fsl-setup">
              <h2 className="fsl-setup__title">Your first connection</h2>
              <ol className="fsl-setup__steps">
                <li>
                  <span className="fsl-setup__number" aria-hidden="true">1</span>
                  <div><h3>Connect EuroScope</h3><p>Install the plugin and connect on your controller callsign and frequency.</p><TextLink label="Plugin setup" href={`${SITE.docs}/getting-started/es-plugin/`} arrow /></div>
                </li>
                <li>
                  <span className="fsl-setup__number" aria-hidden="true">2</span>
                  <div><h3>Use the same VATSIM account</h3><p>Sign in to the plugin and this browser. FlightStrips matches them using your CID.</p><TextLink label="Start here" href={`${SITE.docs}/getting-started/intro/`} arrow /></div>
                </li>
                <li>
                  <span className="fsl-setup__number" aria-hidden="true">3</span>
                  <div><h3>Check your board</h3><p>Verify your layout, runway setup and shared strips before working traffic.</p><TextLink label="First-session checklist" href={`${SITE.docs}/getting-started/first-session/`} arrow /></div>
                </li>
              </ol>
            </div>
          </div>
        </section>

        {/* ── What it plugs into ────────────────────────────── */}
        <Section theme="dark" className="py-10">
          <ul className="grid gap-x-8 gap-y-6 sm:grid-cols-2 lg:grid-cols-5">
            {CONNECTIONS.map((connection) => (
              <li key={connection.label}>
                <p className="mb-2 text-sm font-medium text-[var(--fsl-brand-ink)]">
                  {connection.label}
                </p>
                <p className="text-[13px] leading-snug text-[var(--fsl-ink-muted)]">{connection.detail}</p>
              </li>
            ))}
          </ul>
        </Section>

        {/* ── Establishing plate ────────────────────────────── */}
        <Section theme="dark" topRule={false} className="pb-16 pt-4 sm:pb-20">
          <div>
            <PhotoPlate {...PHOTOS.ops} className="fsl-plate--wide" focus="center 14%" />
          </div>
        </Section>

        {/* ── Architecture ──────────────────────────────────── */}
        <Section id="architecture" theme="light" guides className="py-16 sm:py-24">
          <div>
            <Eyebrow className="mb-5">Architecture</Eyebrow>
            <h2 className="fsl-display mb-6 max-w-[18ch] text-[32px] sm:text-[40px] lg:text-[48px]">
              EuroScope and your board, connected.
            </h2>
            <p className="mb-14 max-w-2xl text-[17px] leading-relaxed text-[var(--fsl-ink-muted)]">
              The plugin supplies the live traffic. You work the strips in your browser. The server keeps
              every connected position in the same session, with one shared view of ownership and coordination.
            </p>
          </div>

          <div>
            <SystemDiagram />
          </div>
        </Section>

        {/* ── Positions ─────────────────────────────────────── */}
        <Section id="positions" theme="dark" className="py-16 sm:py-24">
          <div>
            <Eyebrow className="mb-5">Positions</Eyebrow>
            <h2 className="fsl-display mb-6 max-w-[20ch] text-[32px] sm:text-[40px] lg:text-[48px]">
              Every position gets the board it actually needs.
            </h2>
            <p className="mb-14 max-w-2xl text-[17px] leading-relaxed text-[var(--fsl-ink-muted)]">
              Layouts, bays and ownership routes come from the airport configuration, so a strip arrives
              where the procedure says it should rather than where someone dragged it.
            </p>
          </div>

          <PositionExplorer />
        </Section>

        {/* ── Ownership ─────────────────────────────────────── */}
        <Section theme="light" guides className="py-16 sm:py-24">
          <div className="grid gap-12 lg:grid-cols-2 lg:gap-20">
            <div>
              <Eyebrow className="mb-5">Ownership</Eyebrow>
              <h2 className="fsl-display mb-6 text-[36px] sm:text-[48px] lg:text-[56px]">
                Who holds this strip, and who has it next.
              </h2>
              <p className="text-[17px] leading-relaxed text-[var(--fsl-ink-muted)]">
                The indicator on the left of every strip encodes your position&rsquo;s relationship to it.
                A split box means a coordination is waiting on someone — and every board in the session
                is reading the same answer.
              </p>
            </div>

            <div>
              <dl className="border-t border-[var(--fsl-line)]">
                {OWNERSHIP.map((state) => (
                  <div key={state.title} className="flex gap-5 border-b border-[var(--fsl-line)] py-6">
                    <span
                      aria-hidden="true"
                      className="mt-1 h-6 w-6 shrink-0 border border-[var(--fsl-line-strong)]"
                      style={{ background: state.swatch }}
                    />
                    <div>
                      <dt className="mb-1.5 font-medium">{state.title}</dt>
                      <dd className="text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]">{state.body}</dd>
                    </div>
                  </div>
                ))}
              </dl>
            </div>
          </div>
        </Section>

        {/* ── Capability narrative ──────────────────────────── */}
        <Section theme="dark" className="py-16 sm:py-24">
          <CapabilityStory />
        </Section>

        {/* ── In the room ───────────────────────────────────── */}
        <Section theme="dark" className="py-16 sm:py-24">
          <div>
            <Eyebrow className="mb-5">In the room</Eyebrow>
            <h2 className="fsl-display mb-6 max-w-[18ch] text-[32px] sm:text-[40px] lg:text-[48px]">
              Built by the people working the position.
            </h2>
            <p className="mb-14 max-w-2xl text-[17px] leading-relaxed text-[var(--fsl-ink-muted)]">
              FlightStrips is written in the open by controllers who use it. The workflows on the board
              are the ones the position actually runs, and the system stays out of the way of the
              judgement calls that are yours to make.
            </p>
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <div>
              <PhotoPlate {...PHOTOS.field} focus="center 45%" />
            </div>
            <div>
              <PhotoPlate {...PHOTOS.together} focus="center 40%" />
            </div>
          </div>
        </Section>

        {/* ── Airports ──────────────────────────────────────── */}
        <Section id="airports" theme="light" guides className="py-16 sm:py-24">
          <div>
            <Eyebrow className="mb-5">{SCOPE.eyebrow}</Eyebrow>
            <h2 className="fsl-display mb-14 max-w-[16ch] text-[32px] sm:text-[40px] lg:text-[48px]">
              {SCOPE.title}
            </h2>
          </div>

          <div className="grid border-t border-[var(--fsl-line)] lg:grid-cols-2">
            <div className="border-b border-[var(--fsl-line)] bg-[var(--fsl-surface)] p-8 sm:p-10 lg:border-r">
              <p className="fsl-eyebrow mb-4 text-[var(--fsl-brand-ink)]">
                Live
              </p>
              <h3 className="fsl-display mb-4 text-[26px] sm:text-[32px]">{SCOPE.live.heading}</h3>
              <p className="mb-6 text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]">{SCOPE.live.body}</p>
              <ul className="space-y-2.5">
                {SCOPE.live.points.map((point) => (
                  <li key={point} className="flex gap-3 text-sm leading-relaxed">
                    <span aria-hidden="true" className="mt-[7px] h-1 w-1 shrink-0 bg-[var(--fsl-brand-ink)]" />
                    <span>{point}</span>
                  </li>
                ))}
              </ul>
            </div>

            <div className="border-b border-[var(--fsl-line)] p-8 sm:p-10">
              <p className="fsl-eyebrow mb-4 text-[var(--fsl-ink-muted)]">
                Planned — not built yet
              </p>
              <h3 className="fsl-display mb-4 text-[26px] sm:text-[32px]">{SCOPE.planned.heading}</h3>
              <p className="mb-6 text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]">
                {SCOPE.planned.body}
              </p>
              <ul className="space-y-2.5">
                {SCOPE.planned.points.map((point) => (
                  <li key={point} className="flex gap-3 text-sm leading-relaxed text-[var(--fsl-ink-muted)]">
                    <span
                      aria-hidden="true"
                      className="mt-[7px] h-1 w-1 shrink-0 border border-[var(--fsl-line-strong)]"
                    />
                    <span>{point}</span>
                  </li>
                ))}
              </ul>
            </div>
          </div>

          <div>
            <div className="border-b border-l border-r border-[var(--fsl-line)] bg-[var(--fsl-raised)] p-8 sm:p-10">
              <div className="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
                <div>
                  <h3 className="fsl-display mb-3 text-[26px] sm:text-[32px]">{SCOPE.onboarding.heading}</h3>
                  <p className="max-w-2xl text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]">
                    {SCOPE.onboarding.body}
                  </p>
                </div>
                <ActionLink
                  label={`Email ${SITE.email}`}
                  href={`mailto:${SITE.email}`}
                  variant="secondary"
                  className="shrink-0"
                />
              </div>
            </div>
          </div>
        </Section>

        {/* ── Dual CTA ──────────────────────────────────────── */}
        {/* Full-bleed on purpose: the two cells meet at a single hairline. */}
        <section data-fsl-theme="dark">
          <div className="grid border-t border-[var(--fsl-line)] lg:grid-cols-2">
            <div className="border-b border-[var(--fsl-line)] px-6 py-16 sm:px-12 sm:py-20 lg:border-b-0 lg:border-r">
              <Eyebrow className="mb-5">Controllers</Eyebrow>
              <h2 className="fsl-display mb-5 max-w-[16ch] text-[30px] sm:text-[40px]">
                Sign in, load the plugin, work the strips.
              </h2>
              <p className="mb-8 max-w-md text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]">
                Free for VATSIM controllers. Sign in with the same VATSIM account in the plugin and the
                browser, and the server matches them for you.
              </p>
              <PrimaryAction />
            </div>

            <div className="px-6 py-16 sm:px-12 sm:py-20">
              <Eyebrow className="mb-5">Everyone else</Eyebrow>
              <h2 className="fsl-display mb-5 max-w-[16ch] text-[30px] sm:text-[40px]">
                Read how it works, or read the code.
              </h2>
              <p className="mb-8 max-w-md text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]">
                The documentation covers every position and procedure. The project is open source, and
                contributions from controllers and developers are welcome.
              </p>
              <div className="flex flex-wrap items-start gap-3">
                <ActionLink label="Documentation" href={SITE.docs} variant="secondary" external />
                <ActionLink label="GitHub" href={SITE.github} variant="secondary" external />
              </div>
            </div>
          </div>
        </section>
      </main>

      <LandingFooter />
    </div>
  );
}
