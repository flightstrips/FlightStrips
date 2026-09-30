import { PageHero, PublicPage } from "@/components/public/SiteChrome";
import { Faq } from "@/components/blocks/Faq";

export default function FaqPage() {
  return (
    <PublicPage>
      <PageHero eyebrow="FAQ" title="Got questions?" />
      <Faq />
    </PublicPage>
  );
}
