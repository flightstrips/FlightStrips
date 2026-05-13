import { useState } from "react";
import { Link } from "react-router";
import { useAuth0 } from "@auth0/auth0-react";
import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";

type TabName = "clr" | "apn" | "gnd" | "twr" | "seq";

interface TabData {
  title: string;
  desc: string;
  heading: string;
  products: string[];
  integrationsTitle: string;
  integrations: string;
}

const platformTabs: Record<TabName, TabData> = {
  clr: {
    title: "Clearance Delivery",
    desc: "Issue clearances with built-in PDC support. Validate flight plans, check routes, and coordinate with apron — all from a single bay with the right context for each strip.",
    heading: "Workflows",
    products: ["Pre-departure clearance (PDC)", "Voice clearance issuance", "Validation status tracking", "Route checking & VATSIM SSO", "Handoff to apron"],
    integrationsTitle: "Coordination",
    integrations: "Strip ownership, next/previous controllers, tag requests (REQ), force assume — all visible at a glance through the SI indicator.",
  },
  apn: {
    title: "Apron",
    desc: "Manage pushback, startup, and taxi-out coordination on the apron. Dedicated bays for departures (APN DEP) and arrivals (APN ARR) keep both flows organized without cross-talk.",
    heading: "Workflows",
    products: ["Pushback approval", "Startup clearance", "Stand assignment & changes", "Taxi route to ground handoff", "Arrival stand coordination"],
    integrationsTitle: "Bays available",
    integrations: "APN DEP and APN ARR run independently. Strips transfer cleanly between apron, ground, and tower as aircraft progress through the airfield.",
  },
  gnd: {
    title: "Ground",
    desc: "Taxi clearances, runway crossings, and tower coordination. Ground east (GE) and ground west (GW) split the airfield so each controller works a focused area.",
    heading: "Workflows",
    products: ["Taxi clearance issuance", "Runway crossing coordination", "Hot spot management", "Sequencing to departure runway", "Hand-off to tower"],
    integrationsTitle: "Bays available",
    integrations: "GE + GW for ground operations. Strips flow naturally through ownership transfers as aircraft cross between sectors.",
  },
  twr: {
    title: "Tower",
    desc: "Departures, landings, and runway integration. TE/TW split tower duties on busy fields, but a built-in bandbox mode lets you cover the whole airfield when working alone.",
    heading: "Workflows",
    products: ["Departure clearance & line-up", "Landing clearance & exit", "Runway sequencing", "Go-around coordination", "Bandbox mode (solo tower)"],
    integrationsTitle: "Bays available",
    integrations: "TE + TW for split tower. Alone as Tower procedures document how to run a one-position bandbox cleanly when no ground or apron is online.",
  },
  seq: {
    title: "Sequence Planner",
    desc: "The SEQ PLN view gives you a runway-ordered picture of departures. Re-sequence with drag-and-drop, see TSAT/CTOT pressure at a glance, and coordinate with the tower.",
    heading: "Workflows",
    products: ["Departure runway sequencing", "TSAT / CTOT visualization", "Slot pressure management", "AA + AD integration", "Tower handoff coordination"],
    integrationsTitle: "Related bays",
    integrations: "AA + AD provide arrival/departure overview. SEQ PLN feeds directly into TE and TW for clean tower coordination.",
  },
};

interface FlightStripProps {
  siColor?: "white" | "purple" | "orange";
  position?: string;
  nextPosition?: string;
}

interface StripData {
  callsign: string;
  aircraft: string;
  registration: string;
  stand: string;
  timeLabel: string;
  timeValue: string;
  runway: string;
  color: string;
}

const stripData: Record<string, StripData> = {
  white: {
    callsign: "SAS1234",
    aircraft: "A320",
    registration: "OY-KAU",
    stand: "B19",
    timeLabel: "TSAT",
    timeValue: "1524",
    runway: "22R",
    color: "bg-[#d4c878]",
  },
  purple: {
    callsign: "KLM9015",
    aircraft: "B738",
    registration: "PH-BCD",
    stand: "C42",
    timeLabel: "EOBT",
    timeValue: "1542",
    runway: "04R",
    color: "bg-[#c89090]",
  },
  orange: {
    callsign: "DLH4FX",
    aircraft: "A321",
    registration: "D-AIDL",
    stand: "A07",
    timeLabel: "ARR",
    timeValue: "22L",
    runway: "22L",
    color: "bg-[#7090b0]",
  },
};

function FlightStrip({ siColor = "white", position = "TE", nextPosition }: FlightStripProps) {
  const data = stripData[siColor];

  const siColors: Record<string, string> = {
    white: "bg-white text-gray-900",
    purple: "bg-[#b890d0] text-white",
    orange: "bg-[#e8a060] text-white",
  };

  return (
    <div className={`flex min-w-[320px] rounded-sm overflow-hidden shadow-xl ${data.color}`}>
      {/* SI Box - Split for transfer visualization */}
      {nextPosition ? (
        <div className="flex w-10 flex-shrink-0 overflow-hidden">
          <div className={`flex-1 flex items-center justify-center font-bold text-[10px] ${siColors[siColor]}`}>
            {position}
          </div>
          <div className="flex-1 flex items-center justify-center font-bold text-[10px] bg-[#b890d0] text-white border-l border-black/10">
            {nextPosition}
          </div>
        </div>
      ) : (
        <div className={`flex items-center justify-center font-bold text-xs w-10 flex-shrink-0 ${siColors[siColor]}`}>
          {position}
        </div>
      )}

      {/* Main Content */}
      <div className="flex-1 px-3 py-2 text-[11px] font-mono border-r border-black/10 flex flex-col justify-center">
        <div className="font-bold text-[12px] text-gray-900 mb-1">{data.callsign}</div>
        <div className="text-[10px] text-gray-700 opacity-90 leading-tight">
          {data.aircraft} {data.registration}
        </div>
      </div>

      {/* Stand/Route */}
      <div className="px-3 py-2 border-r border-black/10 flex flex-col justify-center">
        <div className="font-bold text-sm text-gray-900">{data.stand}</div>
        <div className="text-[9px] text-gray-700 mt-1">{data.timeLabel}</div>
      </div>

      {/* Time */}
      <div className="px-2.5 py-2 border-r border-black/10 flex items-center justify-center">
        <div className="font-bold text-[11px] text-gray-900">{data.timeValue}</div>
      </div>

      {/* Runway */}
      <div className="px-3 py-2 flex items-center justify-center">
        <div className="font-bold text-sm text-gray-900">{data.runway}</div>
      </div>
    </div>
  );
}

function SIBoxDemo() {
  const swatches = [
    { color: "white", title: "White — You are the owner", desc: "You currently control the strip. Actions like TRF (transfer) and command bar functions are enabled." },
    { color: "purple", title: "Purple — Next up", desc: "You are in next_controllers but not yet the owner. Click to assume directly when the strip is unowned, or accept a pending handoff." },
    { color: "orange", title: "Orange — Transferred away", desc: "Your position has already held this strip. It's still visible for context, but no longer your responsibility." },
    { color: "grey", title: "Grey — Unconcerned", desc: "You're neither owner, next, nor previous. The strip is on your board for situational awareness only." },
  ];

  return (
    <section className="bg-[#051415] py-20 px-[60px] border-b border-[#233434]">
      <div className="max-w-4xl mx-auto">
        <div className="text-center mb-16">
          <h2 className="text-4xl font-semibold mb-5 tracking-tight">Ownership at a glance.</h2>
          <p className="text-base text-[#8a9a9a] leading-relaxed max-w-2xl mx-auto">
            The SI indicator visualizes how your position relates to every strip on the board — so you always know who controls what, what's coming next, and what's already moved on.
          </p>
        </div>

        <div className="grid md:grid-cols-2 gap-12 items-center">
          <div className="space-y-8">
            {swatches.map((swatch, idx) => (
              <div key={idx} className="flex gap-4 items-start">
                <div
                  className={`w-7 h-7 rounded flex-shrink-0 mt-0.5 ${
                    swatch.color === "white" ? "bg-white" : swatch.color === "purple" ? "bg-[#b890d0]" : swatch.color === "orange" ? "bg-[#e8a060]" : "bg-[#5a6a6a]"
                  }`}
                />
                <div>
                  <h3 className="text-base font-semibold text-white mb-1.5">{swatch.title}</h3>
                  <p className="text-sm text-[#8a9a9a] leading-relaxed">{swatch.desc}</p>
                </div>
              </div>
            ))}
          </div>

          <div className="bg-[#0b1e1e] border border-[#233434] rounded-2xl p-10 space-y-4">
            <div className="text-xs text-[#8a9a9a] font-mono uppercase tracking-wider mb-2">Your perspective</div>
            <FlightStrip siColor="white" position="TE" />
            <div className="text-xs text-[#8a9a9a] font-mono uppercase tracking-wider mt-4 mb-2">Transfer to GE </div>
            <FlightStrip siColor="white" position="TE" nextPosition="GE" />
          </div>
        </div>
      </div>
    </section>
  );
}

export default function Home() {
  const { isAuthenticated, loginWithRedirect } = useAuth0();
  const [activeTab, setActiveTab] = useState<TabName>("clr");
  const currentTabData = platformTabs[activeTab];

  return (
    <div className="bg-[#254a54] text-white min-h-screen flex flex-col relative" style={{ backgroundImage: "radial-gradient(circle, #1a2a2a 1.2px, transparent 1.2px)", backgroundSize: "32px 32px" }}>
      {/* Top Banner */}
      <div className="hidden md:flex bg-[#254a54] px-[60px] py-3.5 justify-between items-center text-sm relative z-10">
        <div />
        <div className="text-center flex-1">
          <a href="https://flightstrips.dk/app" className="text-white hover:text-[#a0dae4] transition-colors">
            Alpha testing is now open →
          </a>
        </div>
        <div className="flex gap-5.5 text-sm">
          <a href="https://docs.flightstrips.dk" className="text-white hover:text-[#a0dae4] transition-colors">
            Docs
          </a>
          <a href="https://github.com/flightstrips" className="text-white hover:text-[#a0dae4] transition-colors">
            GitHub
          </a>
          <a href="https://docs.flightstrips.dk/troubleshooting/authentication-port-conflicts/" className="text-white hover:text-[#a0dae4] transition-colors">
            Support
          </a>
        </div>
      </div>

      {/* Header */}
      <PublicNavigation linkTone="landing" className="px-6 md:px-[60px]" />

      <main className="flex-1 relative z-1">
        {/* Hero Section */}
        <section className="bg-[#051415] text-center py-28 px-[60px]">
          <div className="max-w-3xl mx-auto mb-12">
            <div className="inline-flex items-center gap-2 px-3.5 py-1.5 border border-[#233434] rounded-full text-xs font-mono text-[#a0dae4] mb-7 bg-[#0b1e1e]/60">
              <span className="w-1.5 h-1.5 bg-[#a0dae4] rounded-full shadow-[0_0_8px_#a0dae4] animate-pulse" />
              BUILT FOR VATSIM CONTROLLERS
            </div>
            <h1 className="text-5xl font-semibold mb-4.5 tracking-tighter leading-tight">The FlightStrips board, reimagined.</h1>
            <p className="text-base text-[#8a9a9a] mb-9 max-w-xl mx-auto leading-relaxed">
              A modern, web-based strip board for tower and ground operations. Coordinated ownership, real-time sync, and EuroScope integration — purpose-built for VATSIM controllers.
            </p>

            <div className="flex gap-3.5 justify-center mb-20">
              <button className="bg-[#a0dae4] text-[#051415] px-7 py-[11px] rounded-full text-sm font-medium hover:bg-[#b8e3ec] transition-colors">
                Open the board
              </button>
              <button className="bg-transparent text-white px-7 py-[11px] border border-[#3a4a4a] rounded-full text-sm font-medium hover:border-[#a0dae4] hover:text-[#a0dae4] transition-colors">
                Read the docs
              </button>
            </div>

            <p className="text-sm text-[#8a9a9a]">Currently live for EKCH Kastrup. More airports coming soon.</p>
          </div>

          {/* Flight Strip Preview */}
          <div className="flex justify-center items-center gap-3 flex-wrap max-w-4xl mx-auto">
            <FlightStrip siColor="white" position="TE" />
            <FlightStrip siColor="purple" position="GE" />
            <FlightStrip siColor="orange" position="AA" />
          </div>
        </section>

        {/* Stats Section */}
        <section className="bg-[#051415] py-32 px-[60px] border-t border-[#233434]">
          <div className="max-w-5xl mx-auto">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              {[
                {
                  number: "~0ms",
                  label: "strip sync latency",
                  desc: "Real-time WebSocket coordination keeps every controller's board in sync — handoffs, ownership, and pending transfers update instantly across positions.",
                  footer: "Live coordination",
                },
                {
                  number: "EKCH",
                  label: "Kastrup, live today",
                  desc: "Full airport-specific workflows for Copenhagen: CLR DEL, APRON DEP/ARR, SEQ PLN, AA+AD, GE+GW, TE+TW, and integrated clearance delivery.",
                  footer: "Built for the field",
                },
                {
                  number: "100%",
                  label: "VATSIM-native",
                  desc: "Authenticate with your VATSIM account. Strip board syncs with the EuroScope plugin, so positions, callsigns, and flight plans flow through automatically.",
                  footer: "VATSIM SSO + EuroScope",
                },
              ].map((stat, idx) => (
                <div key={idx} className="bg-[#0b1e1e] border border-[#233434] rounded-xl p-7 flex flex-col min-h-72">
                  <div className="text-5xl font-semibold text-white mb-1 tracking-tighter leading-tight">{stat.number}</div>
                  <div className="text-base font-semibold text-white mb-3.5">{stat.label}</div>
                  <p className="text-sm text-[#8a9a9a] leading-relaxed mb-6 flex-grow">{stat.desc}</p>
                  <div className="flex items-center gap-2.5 pt-4.5 border-t border-[#233434] text-sm font-medium text-white">
                    <span className="text-[#a0dae4]">✓</span>
                    {stat.footer}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* Platform Layers Section */}
        <section className="bg-[#051415] py-20 px-[60px] border-t border-[#233434]">
          <div className="max-w-5xl mx-auto">
            <div className="text-center max-w-2xl mx-auto mb-16">
              <h2 className="text-4xl font-semibold mb-5 tracking-tight">One board. Every position. Every workflow.</h2>
              <p className="text-base text-[#8a9a9a] leading-relaxed">
                From clearance delivery to tower, every position has dedicated bays and procedures — purpose-built for the way controllers actually work. No more juggling paper strips or third-party tools.
              </p>
            </div>

            <div className="grid md:grid-cols-[240px_1fr_1fr] gap-8 items-start">
              {/* Tabs */}
              <div className="flex flex-col gap-1">
                {(["clr", "apn", "gnd", "twr", "seq"] as TabName[]).map((tab) => (
                  <button
                    key={tab}
                    onClick={() => setActiveTab(tab)}
                    className={`flex items-center gap-2.5 px-4 py-2.5 rounded-full text-sm text-left transition-colors font-normal ${
                      activeTab === tab ? "bg-[#a0dae4] text-[#051415] font-medium" : "text-[#8a9a9a] hover:text-white"
                    }`}
                  >
                    <span>→</span>
                    {tab === "clr" && "Clearance Delivery"}
                    {tab === "apn" && "Apron"}
                    {tab === "gnd" && "Ground"}
                    {tab === "twr" && "Tower"}
                    {tab === "seq" && "Sequence Planner"}
                  </button>
                ))}
              </div>

              {/* Visual */}
              <div className="bg-[#0b1e1e] border border-[#233434] rounded-2xl aspect-square flex items-center justify-center p-5 overflow-hidden relative">
                <div
                  className="absolute inset-0 pointer-events-none"
                  style={{
                    background: "radial-gradient(circle at 50% 60%, rgba(160, 218, 228, 0.08) 0%, transparent 60%)",
                  }}
                />
                <div className="flex flex-col gap-1 scale-85 origin-top-left">
                  <FlightStrip siColor="white" position="CLR" />
                  <FlightStrip siColor="purple" position="GE" />
                  <FlightStrip siColor="white" position="TW" />
                </div>
              </div>

              {/* Details */}
              <div>
                <h3 className="text-2xl font-semibold mb-3 tracking-tight">{currentTabData.title}</h3>
                <p className="text-sm text-[#8a9a9a] leading-relaxed mb-6">{currentTabData.desc}</p>

                <h4 className="text-sm font-semibold text-white mb-2.5">{currentTabData.heading}</h4>
                <ul className="mb-6 space-y-0.75">
                  {currentTabData.products.map((product, idx) => (
                    <li key={idx} className="text-sm text-[#8a9a9a] leading-relaxed pl-4.5 relative">
                      <span className="absolute left-1.5 text-[#a0dae4]">•</span>
                      {product}
                    </li>
                  ))}
                </ul>

                <div className="border-t border-[#233434] pt-5">
                  <h4 className="text-sm font-semibold text-white mb-2">{currentTabData.integrationsTitle}</h4>
                  <p className="text-sm text-[#8a9a9a] leading-relaxed">{currentTabData.integrations}</p>
                </div>
              </div>
            </div>

            <div className="text-center mt-16">
              <a
                href="https://docs.flightstrips.dk"
                className="inline-flex items-center gap-2 bg-transparent text-white px-8 py-[11px] border border-[#233434] rounded-full text-sm font-medium hover:border-[#a0dae4] hover:text-[#a0dae4] transition-colors"
              >
                Browse all procedures
                <span>→</span>
              </a>
            </div>
          </div>
        </section>

        {/* SI Box Section */}
        <SIBoxDemo />

        {/* Resources Section */}
        <section className="bg-[#051415] py-20 px-[60px] border-t border-[#233434]">
          <div className="max-w-5xl mx-auto">
            <div className="flex justify-between items-center mb-8">
              <div className="flex items-center gap-4">
                <h2 className="text-4xl font-semibold tracking-tight">Documentation</h2>
                <a href="https://docs.flightstrips.dk" className="text-white text-sm px-4 py-1.5 border border-[#233434] rounded-full hover:border-[#a0dae4] hover:text-[#a0dae4] transition-colors">
                  View all
                </a>
              </div>
              <div className="flex gap-2">
                <button className="w-8 h-8 border border-[#233434] rounded-full flex items-center justify-center text-white hover:bg-white/10 transition-colors">
                  ←
                </button>
                <button className="w-8 h-8 border border-[#233434] rounded-full flex items-center justify-center bg-[#a0dae4] text-[#051415] font-bold">
                  →
                </button>
              </div>
            </div>

            <div className="grid md:grid-cols-3 gap-4">
              {[
                {
                  type: "Getting Started",
                  title: "Introduction & first-time setup for FlightStrips",
                  date: "Getting Started",
                  time: "5 min read",
                  href: "https://docs.flightstrips.dk/getting-started/intro/",
                },
                {
                  type: "Concepts",
                  title: "Strip ownership, handoffs, and the SI indicator explained",
                  date: "Concepts",
                  time: "8 min read",
                  href: "https://docs.flightstrips.dk/concepts/ownership/",
                },
                {
                  type: "Kastrup",
                  title: "CLR DEL: Working clearance delivery at EKCH",
                  date: "EKCH",
                  time: "10 min read",
                  href: "https://docs.flightstrips.dk/ekch/clr-del/",
                },
              ].map((resource, idx) => (
                <a
                  key={idx}
                  href={resource.href}
                  className="bg-[#0b1e1e] border border-[#233434] rounded-xl p-6 flex flex-col min-h-56 hover:border-[#a0dae4] hover:translate-y-[-2px] transition-all group"
                >
                  <div className="text-xs text-[#a0dae4] font-mono uppercase tracking-wider mb-6">
                    <span>→ {resource.type}</span>
                  </div>
                  <h3 className="text-lg font-semibold text-white leading-snug mb-4 flex-grow group-hover:text-[#a0dae4] transition-colors">
                    {resource.title}
                  </h3>
                  <div className="text-xs text-[#8a9a9a] mb-4 flex items-center gap-2">
                    <span>{resource.date}</span>
                    <span className="w-0.5 h-0.5 bg-[#8a9a9a] rounded-full" />
                    <span>{resource.time}</span>
                  </div>
                  <span className="text-white text-sm font-medium">Read →</span>
                </a>
              ))}
            </div>
          </div>
        </section>

        {/* CTA Section */}
        <section className="bg-[#051415] py-24 px-[60px] border-t border-[#233434]">
          <div className="max-w-5xl mx-auto">
            <div
              className="bg-[#0b1e1e] border border-[#233434] rounded-3xl py-20 px-10 text-center relative overflow-hidden"
              style={{
                background: "linear-gradient(180deg, #0b1e1e 0%, #0a1818 100%)",
              }}
            >
              <div
                className="absolute bottom-[-40%] left-1/2 w-[80%] h-[80%] -translate-x-1/2 rounded-full pointer-events-none"
                style={{
                  background: "radial-gradient(ellipse at center, rgba(160, 218, 228, 0.3) 0%, rgba(0, 61, 72, 0.2) 30%, transparent 60%)",
                }}
              />
              <div
                className="absolute inset-0 pointer-events-none"
                style={{
                  backgroundImage: "radial-gradient(circle, rgba(255,255,255,0.04) 1px, transparent 1px)",
                  backgroundSize: "24px 24px",
                }}
              />

              <div className="relative z-10">
                <h2 className="text-3xl font-semibold mb-4 tracking-tight">Ready for your next session?</h2>
                <p className="text-sm text-[#8a9a9a] max-w-xl mx-auto mb-7 leading-relaxed">
                  Sign in with VATSIM, connect the EuroScope plugin, and start working strips. Free for all VATSIM controllers, forever.
                </p>
                {isAuthenticated ? (
                  <Link
                    to="/app"
                    className="inline-flex items-center justify-center bg-[#a0dae4] text-[#051415] px-8 py-[11px] rounded-full text-sm font-medium hover:bg-[#b8e3ec] transition-colors"
                  >
                    Open App
                  </Link>
                ) : (
                  <button
                    type="button"
                    onClick={() => loginWithRedirect()}
                    className="bg-[#a0dae4] text-[#051415] px-8 py-[11px] rounded-full text-sm font-medium hover:bg-[#b8e3ec] transition-colors"
                  >
                    Log in
                  </button>
                )}
              </div>
            </div>
          </div>
        </section>
      </main>

      <PublicFooter tone="landing" />
    </div>
  );
}
