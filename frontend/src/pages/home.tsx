import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { ScrollProgress } from "@/components/public/ScrollProgress";
import { Hero } from "@/components/blocks/Hero";
import { Features } from "@/components/blocks/Features";
import { Logos } from "@/components/blocks/Logos";
import { Faq } from "@/components/blocks/Faq";
import { CtaSection } from "@/components/blocks/CtaSection";

/**
 * Home page built with Mainline-style blocks (shadcn/ui template).
 * FlightStrips palette: cream, navy, primary (green #003d48).
 * @see https://github.com/shadcnblocks/mainline-nextjs-template
 */
export default function Home() {
  return (
    <div className="min-h-screen bg-cream text-navy">
      <ScrollProgress />
      <PublicNavigation />
      <Hero />
      <Features />
      <Logos />
      <Faq />
      <CtaSection />
      <PublicFooter />
    </div>
  );
}
