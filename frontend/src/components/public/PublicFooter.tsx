import { Link } from "react-router";
import { publicNavLinks } from "./publicNavLinks";

export function PublicFooter() {
  return (
    <footer className="mt-auto border-t border-cream/10 dark:border-white/10 bg-navy dark:bg-card text-cream dark:text-foreground py-8 px-6 md:px-8">
      <div className="max-w-4xl mx-auto flex flex-col items-center gap-6 text-center">
        <nav className="flex flex-wrap items-center justify-center gap-x-6 gap-y-1 text-sm">
          {publicNavLinks.map((link) =>
            "external" in link && link.external ? (
              <a
                key={link.to + link.label}
                href={link.to}
                target="_blank"
                rel="noopener noreferrer"
                className="text-cream/80 dark:text-foreground/80 hover:text-cream dark:hover:text-foreground transition-colors"
              >
                {link.label}
              </a>
            ) : (
              <Link
                key={link.to + link.label}
                to={link.to}
                className="text-cream/80 dark:text-foreground/80 hover:text-cream dark:hover:text-foreground transition-colors"
              >
                {link.label}
              </Link>
            )
          )}
          <Link
            to="/privacy"
            className="text-cream/80 dark:text-foreground/80 hover:text-cream dark:hover:text-foreground transition-colors"
          >
            Privacy
          </Link>
          <Link
            to="/data-handling"
            className="text-cream/80 dark:text-foreground/80 hover:text-cream dark:hover:text-foreground transition-colors"
          >
            Data Handling
          </Link>
        </nav>
        <p className="text-xs text-cream/60 dark:text-foreground/60">
          For simulation use only. Not for real-world operations.
        </p>
      </div>
    </footer>
  );
}
