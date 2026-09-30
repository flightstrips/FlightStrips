import "./landing/landing.css";

import { LandingFooter } from "./landing/LandingFooter";
import { LandingHeader } from "./landing/LandingHeader";

/**
 * The landing page's header and footer for the other public pages. Those pages
 * keep their own body styling; `.fsl` only supplies the design tokens.
 */
export function SiteHeader() {
  // `contents` so the wrapper does not become the sticky header's scroll box.
  return (
    <div className="fsl contents">
      <LandingHeader home={false} />
    </div>
  );
}

export function SiteFooter() {
  return (
    <div className="fsl mt-auto">
      <LandingFooter />
    </div>
  );
}
