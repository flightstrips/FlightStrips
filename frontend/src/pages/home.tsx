import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";

export default function Home() {
  return (
    <div className="min-h-screen bg-cream text-navy flex flex-col">
      <PublicNavigation />
      <main className="flex-1" />
      <PublicFooter />
    </div>
  );
}
