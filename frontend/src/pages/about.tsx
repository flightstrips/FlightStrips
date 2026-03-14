import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { ScrollProgress } from "@/components/public/ScrollProgress";
import { AboutHero } from "@/components/blocks/AboutHero";
import { AboutContent } from "@/components/blocks/AboutContent";

/**
 * About page built with Mainline-style blocks.
 * @see https://github.com/shadcnblocks/mainline-nextjs-template
 */
export default function About() {
  return (
    <div className="min-h-screen bg-cream text-navy flex flex-col">
      <ScrollProgress />
      <PublicNavigation />
      <AboutHero />
      <AboutContent />
      <PublicFooter />
    </div>
  );
}
