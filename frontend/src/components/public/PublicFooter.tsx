import { Link } from "react-router";
import { publicNavLinks } from "./publicNavLinks";
import { cn } from "@/lib/utils";

export type PublicFooterTone = "default" | "industrial";

export function PublicFooter({ tone = "default" }: { tone?: PublicFooterTone }) {
  const linkClass =
    tone === "industrial"
      ? "text-white/65 hover:text-[#ff5a1f] transition-colors"
      : "text-cream/80 dark:text-foreground/80 hover:text-cream dark:hover:text-foreground transition-colors";
  const mutedClass = tone === "industrial" ? "text-white/45" : "text-cream/60 dark:text-foreground/60";

  return (
    <footer
      className={cn(
        "mt-auto border-t py-10 px-6 md:px-8",
        tone === "industrial"
          ? "border-white/10 bg-[#1a1a1a] text-white"
          : "border-cream/10 dark:border-white/10 bg-navy dark:bg-card text-cream dark:text-foreground",
      )}
    >
      <div className="max-w-5xl mx-auto flex flex-col items-center gap-6 text-center">
        <nav className="flex flex-wrap items-center justify-center gap-x-6 gap-y-1 text-[13px] tracking-tight">
          {publicNavLinks.map((link) =>
            "external" in link && link.external ? (
              <a
                key={link.to + link.label}
                href={link.to}
                target="_blank"
                rel="noopener noreferrer"
                className={linkClass}
              >
                {link.label}
              </a>
            ) : (
              <Link key={link.to + link.label} to={link.to} className={linkClass}>
                {link.label}
              </Link>
            )
          )}
          <Link to="/privacy" className={linkClass}>
            Privacy
          </Link>
          <Link to="/data-handling" className={linkClass}>
            Data Handling
          </Link>
        </nav>
        <p className={cn("text-xs", mutedClass)}>For simulation use only. Not for real-world operations.</p>
      </div>
    </footer>
  );
}
