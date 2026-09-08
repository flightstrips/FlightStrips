import { Link } from "react-router";
import "./landing.css";

import { CONNECTIONS, HERO, OWNERSHIP, PHOTOS, SCOPE, SITE } from "./content";
import { CapabilityStory } from "./CapabilityStory";
import { LandingFooter } from "./LandingFooter";
import { LandingHeader } from "./LandingHeader";
import { PhotoPlate } from "./PhotoPlate";
import { PositionExplorer } from "./PositionExplorer";
import { Eyebrow, GuideColumns, Reveal, Section, SwapButton, SwapLink } from "./primitives";
import { ArrivalSpecimen, ClearedSpecimen } from "./StripSpecimen";
import { SystemDiagram } from "./SystemDiagram";
import { useLandingAuth } from "./useLandingAuth";

function PrimaryAction({ label = "Open the board" }: { label?: string }) {
  const { isAuthenticated, signIn } = useLandingAuth();

  if (isAuthenticated) {
    return (
      <Link to="/app" className="fsl-swap fsl-swap--primary">
        <span className="fsl-swap__face fsl-swap__face--rest">{label}</span>
        <span className="fsl-swap__face fsl-swap__face--hover" aria-hidden="true">
          {label}
        </span>
      </Link>
    );
  }

  return <SwapButton label="Sign in with VATSIM" onClick={signIn} />;
}

export function LandingPage() {
  return (
    <div className="fsl min-h-screen">
      <LandingHeader />

      <main>
        {/* ── Hero ──────────────────────────────────────────── */}
        <section data-fsl-theme="dark" className="relative isolate overflow-hidden">
          <div className="fsl-ambient" aria-hidden="true" />
          <GuideColumns />

          <div className="fsl-gutter relative z-10 pb-20 pt-16 sm:pb-28 sm:pt-24 lg:pb-36 lg:pt-32">
            <div className="fsl-grid">
              <div className="col-span-8 lg:col-span-10">
                <Reveal>
                  <div className="mb-8 inline-flex items-center gap-2.5 border border-[var(--fsl-line-strong)] px-3.5 py-2">
                    <span
                      aria-hidden="true"
                      className="h-1.5 w-1.5 rounded-full bg-[var(--fsl-accent)] shadow-[0_0_8px_var(--fsl-accent)]"
                    />
                    <span className="fsl-mono text-[11px] uppercase tracking-[0.18em]">{HERO.badge}</span>
                  </div>

                  <h1 className="fsl-display mb-7 max-w-[13ch] text-[42px] sm:text-[64px] lg:text-[84px] xl:text-[96px]">
                    {HERO.headline}
                  </h1>
                </Reveal>

                <Reveal delay={120}>
                  <p className="mb-10 max-w-2xl text-[17px] leading-relaxed text-[var(--fsl-ink-muted)] sm:text-[19px]">
                    {HERO.standfirst}
                  </p>

                  <div className="flex flex-wrap gap-3">
                    <PrimaryAction />
                    <SwapLink label="Read the docs" href={SITE.docs} variant="secondary" external />
                  </div>

                  <p className="fsl-mono mt-8 text-[11px] uppercase tracking-[0.18em] text-[var(--fsl-ink-muted)]">
                    {HERO.note}
                  </p>
                </Reveal>
              </div>
            </div>

            {/* Strip stack — the product's own object rendered 1:1, not a screenshot. */}
            <Reveal delay={300} className="mt-14 lg:mt-20">
              <div className="fsl-grid">
                <div className="col-span-8 flex max-w-[640px] flex-col gap-3 lg:col-span-9 lg:col-start-7 lg:max-w-none">
                  <ClearedSpecimen
                    si="assumed"
                    nextLabel="A"
                    callsign="SAS1462"
                    destination="ESSA"
                    stand="B19"
                    eobt="1515"
                    tobt="1518"
                    tsat="1524"
                  />
                  <ClearedSpecimen
                    si="sending"
                    nextLabel="A"
                    callsign="KLM18X"
                    destination="EHAM"
                    stand="C42"
                    eobt="1530"
                    ctot="1547"
                    tobt="1533"
                    tsat="1541"
                    tsatIssued={false}
                    className="lg:ml-[5%]"
                  />
                  <ArrivalSpecimen
                    si="concerned"
                    callsign="DLH820"
                    aircraftType="A21N"
                    registration="D-AIEA"
                    runway="22L"
                    taxiway="M8"
                    stand="A07"
                    className="lg:ml-[10%]"
                  />
                </div>
              </div>
            </Reveal>
          </div>
        </section>

        {/* ── What it plugs into ────────────────────────────── */}
        <Section theme="dark" className="py-10">
          <ul className="grid gap-x-8 gap-y-6 sm:grid-cols-2 lg:grid-cols-5">
            {CONNECTIONS.map((connection) => (
              <li key={connection.label}>
                <p className="fsl-mono mb-1.5 text-[12px] uppercase tracking-[0.14em] text-[var(--fsl-brand-ink)]">
                  {connection.label}
                </p>
                <p className="text-[13px] leading-snug text-[var(--fsl-ink-muted)]">{connection.detail}</p>
              </li>
            ))}
          </ul>
        </Section>

        {/* ── Establishing plate ────────────────────────────── */}
        <Section theme="dark" topRule={false} className="pb-16 pt-4 sm:pb-20">
          <Reveal>
            <PhotoPlate {...PHOTOS.ops} className="fsl-plate--wide" focus="center 14%" />
          </Reveal>
        </Section>

        {/* ── Architecture ──────────────────────────────────── */}
        <Section id="architecture" theme="light" guides className="py-20 sm:py-28">
          <Reveal>
            <Eyebrow className="mb-5">Architecture</Eyebrow>
            <h2 className="fsl-display mb-6 max-w-[18ch] text-[36px] sm:text-[52px] lg:text-[64px]">
              Three clients, one server, four services.
            </h2>
            <p className="mb-14 max-w-2xl text-[17px] leading-relaxed text-[var(--fsl-ink-muted)]">
              The plugin supplies live EuroScope data, the web app is where strips are operated, and the
              server is the only thing that decides what the session currently looks like.
            </p>
          </Reveal>

          <Reveal delay={100}>
            <SystemDiagram />
          </Reveal>
        </Section>

        {/* ── Positions ─────────────────────────────────────── */}
        <Section id="positions" theme="dark" className="py-20 sm:py-28">
          <Reveal>
            <Eyebrow className="mb-5">Positions</Eyebrow>
            <h2 className="fsl-display mb-6 max-w-[20ch] text-[36px] sm:text-[52px] lg:text-[64px]">
              Every position gets the board it actually needs.
            </h2>
            <p className="mb-14 max-w-2xl text-[17px] leading-relaxed text-[var(--fsl-ink-muted)]">
              Layouts, bays and ownership routes come from the airport configuration, so a strip arrives
              where the procedure says it should rather than where someone dragged it.
            </p>
          </Reveal>

          <PositionExplorer />
        </Section>

        {/* ── Ownership ─────────────────────────────────────── */}
        <Section theme="light" guides className="py-20 sm:py-28">
          <div className="grid gap-12 lg:grid-cols-2 lg:gap-20">
            <Reveal>
              <Eyebrow className="mb-5">Ownership</Eyebrow>
              <h2 className="fsl-display mb-6 text-[36px] sm:text-[48px] lg:text-[56px]">
                Who holds this strip, and who has it next.
              </h2>
              <p className="text-[17px] leading-relaxed text-[var(--fsl-ink-muted)]">
                The indicator on the left of every strip encodes your position&rsquo;s relationship to it.
                A split box means a coordination is waiting on someone — and every board in the session
                is reading the same answer.
              </p>
            </Reveal>

            <Reveal delay={100}>
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
            </Reveal>
          </div>
        </Section>

        {/* ── Capability narrative ──────────────────────────── */}
        <Section theme="dark" className="py-20 sm:py-28">
          <CapabilityStory />
        </Section>

        {/* ── In the room ───────────────────────────────────── */}
        <Section theme="dark" className="py-20 sm:py-28">
          <Reveal>
            <Eyebrow className="mb-5">In the room</Eyebrow>
            <h2 className="fsl-display mb-6 max-w-[18ch] text-[36px] sm:text-[52px] lg:text-[64px]">
              Built by the people working the position.
            </h2>
            <p className="mb-14 max-w-2xl text-[17px] leading-relaxed text-[var(--fsl-ink-muted)]">
              FlightStrips is written in the open by controllers who use it. The workflows on the board
              are the ones the position actually runs, and the system stays out of the way of the
              judgement calls that are yours to make.
            </p>
          </Reveal>

          <div className="grid gap-4 lg:grid-cols-2">
            <Reveal>
              <PhotoPlate {...PHOTOS.field} focus="center 45%" />
            </Reveal>
            <Reveal delay={120}>
              <PhotoPlate {...PHOTOS.together} focus="center 40%" />
            </Reveal>
          </div>
        </Section>

        {/* ── Airports ──────────────────────────────────────── */}
        <Section id="airports" theme="light" guides className="py-20 sm:py-28">
          <Reveal>
            <Eyebrow className="mb-5">{SCOPE.eyebrow}</Eyebrow>
            <h2 className="fsl-display mb-14 max-w-[16ch] text-[36px] sm:text-[52px] lg:text-[64px]">
              {SCOPE.title}
            </h2>
          </Reveal>

          <div className="grid border-t border-[var(--fsl-line)] lg:grid-cols-2">
            <Reveal className="border-b border-[var(--fsl-line)] bg-[var(--fsl-surface)] p-8 sm:p-10 lg:border-r">
              <p className="fsl-mono mb-4 text-[11px] uppercase tracking-[0.18em] text-[var(--fsl-brand-ink)]">
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
            </Reveal>

            <Reveal delay={100} className="border-b border-[var(--fsl-line)] p-8 sm:p-10">
              <p className="fsl-mono mb-4 text-[11px] uppercase tracking-[0.18em] text-[var(--fsl-ink-muted)]">
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
            </Reveal>
          </div>

          <Reveal delay={150}>
            <div className="border-b border-l border-r border-[var(--fsl-line)] bg-[var(--fsl-raised)] p-8 sm:p-10">
              <div className="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
                <div>
                  <h3 className="fsl-display mb-3 text-[26px] sm:text-[32px]">{SCOPE.onboarding.heading}</h3>
                  <p className="max-w-2xl text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]">
                    {SCOPE.onboarding.body}
                  </p>
                </div>
                <SwapLink
                  label={`Email ${SITE.email}`}
                  href={`mailto:${SITE.email}`}
                  variant="secondary"
                  className="shrink-0"
                />
              </div>
            </div>
          </Reveal>
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
              <div className="flex flex-wrap gap-3">
                <SwapLink label="Documentation" href={SITE.docs} variant="secondary" external />
                <SwapLink label="GitHub" href={SITE.github} variant="secondary" external />
              </div>
            </div>
          </div>
        </section>
      </main>

      <LandingFooter />
    </div>
  );
}
