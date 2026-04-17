import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { AboutHero } from "@/components/blocks/AboutHero";
import { AboutContent } from "@/components/blocks/AboutContent";
import { PUBLIC_NAV_INDUSTRY_CLASS, PUBLIC_PAGE_SHELL_CLASS } from "@/lib/public-page-style";

export default function About() {
  return (
    <div className={PUBLIC_PAGE_SHELL_CLASS}>
      <PublicNavigation linkTone="industrial" className={PUBLIC_NAV_INDUSTRY_CLASS} />
      <main className="flex-1 pt-[4.5rem]">
        <AboutHero />
        <AboutContent />
      </main>
      <PublicFooter tone="industrial" />
    </div>
  );
}
