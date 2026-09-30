import { SiteFooter, SiteHeader } from "@/components/public/SiteChrome";
import { AboutHero } from "@/components/blocks/AboutHero";
import { AboutContent } from "@/components/blocks/AboutContent";
import { PUBLIC_PAGE_SHELL_CLASS } from "@/lib/public-page-style";

export default function About() {
  return (
    <div className={PUBLIC_PAGE_SHELL_CLASS}>
      <SiteHeader />
      <main className="flex-1">
        <AboutHero />
        <AboutContent />
      </main>
      <SiteFooter />
    </div>
  );
}
