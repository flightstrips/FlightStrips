import { SiteFooter, SiteHeader } from "@/components/public/SiteChrome";
import { ScrollProgress } from "@/components/public/ScrollProgress";
import { Faq } from "@/components/blocks/Faq";

export default function FaqPage() {
  return (
    <div className="min-h-screen bg-cream dark:bg-background text-navy dark:text-foreground">
      <ScrollProgress />
      <SiteHeader />
      <Faq />
      <SiteFooter />
    </div>
  );
}
