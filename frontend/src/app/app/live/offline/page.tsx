import AAAD from "@/airport/ekch/AAAD";
import CommandBar from "@/components/refactor/commandbar/CommandBar";

export default function LiveOfflinePage() {
  return (
    <div className="relative">
      <AAAD />
      <CommandBar />
    </div>
  );
}
