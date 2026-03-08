import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { ScrollProgress } from "@/components/public/ScrollProgress";
import { Faq } from "@/components/blocks/Faq";

export default function FaqPage() {
  return (
    <div className="min-h-screen bg-cream text-navy">
      <ScrollProgress />
      <PublicNavigation />
      <Faq />
      <PublicFooter />
    </div>
  );
}
