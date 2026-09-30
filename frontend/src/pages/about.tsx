import { PageHero, PublicPage } from "@/components/public/SiteChrome";
import { AboutContent } from "@/components/blocks/AboutContent";
import { AboutStats } from "@/components/blocks/AboutHero";

export default function About() {
  return (
    <PublicPage>
      <PageHero
        eyebrow="About"
        title="Built for virtual ATC"
        lead="FlightStrips brings NITOS-inspired strip management to simulation: precision, clarity, and reliability—on any device."
      >
        <AboutStats />
      </PageHero>
      <AboutContent />
    </PublicPage>
  );
}
