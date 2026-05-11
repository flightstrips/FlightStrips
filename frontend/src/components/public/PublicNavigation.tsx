import { useState } from "react";
import { Link } from "react-router";
import { useAuth0 } from "@auth0/auth0-react";
import { Menu, Sun, Moon } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  getStoredPublicTheme,
  setStoredPublicTheme,
  type PublicTheme,
} from "@/lib/public-theme";
import { cn } from "@/lib/utils";

export type PublicNavigationTone = "landing" | "industrial";

export type PublicNavigationProps = {
  className?: string;
  linkTone?: PublicNavigationTone;
};

export function PublicNavigation({ className, linkTone = "landing" }: PublicNavigationProps) {
  const { isAuthenticated, loginWithRedirect, logout } = useAuth0();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [theme, setTheme] = useState<PublicTheme>(() => getStoredPublicTheme());

  const toggleTheme = () => {
    const next = theme === "light" ? "dark" : "light";
    setStoredPublicTheme(next);
    setTheme(next);
  };

  const navLinks = [
    { label: "Product", href: "#" },
    { label: "Airports", href: "#" },
    { label: "Docs", href: "https://docs.flightstrips.dk" },
    { label: "Resources", href: "#" },
    { label: "Community", href: "#" },
  ];

  if (linkTone === "industrial") {
    return (
      <header
        className={cn(
          "fixed top-0 left-0 right-0 z-40 flex items-center justify-between px-6 py-4 pr-14 md:pr-8 sm:px-8 md:grid md:grid-cols-3 md:justify-items-stretch border-b border-neutral-300/90 dark:border-white/10 bg-[#f5f5f5] dark:bg-neutral-950 backdrop-blur-md",
          className,
        )}
      >
        <Link
          to="/"
          className="font-display font-semibold text-xl tracking-tight text-neutral-900 hover:text-[#ff5a1f] dark:text-neutral-100 dark:hover:text-[#ff5a1f] transition-colors md:justify-self-start"
        >
          FlightStrips
        </Link>

        <div className="hidden md:flex items-center justify-end gap-2">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={toggleTheme}
            aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            className="text-neutral-800 hover:bg-black/[0.04] dark:text-neutral-200"
          >
            {theme === "dark" ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
          </Button>
          {isAuthenticated ? (
            <>
              <Link to="/app" className="hi-bracket text-neutral-900 dark:text-neutral-100">
                Open App
              </Link>
              <button
                type="button"
                className="hi-bracket text-neutral-900 dark:text-neutral-100"
                onClick={() => logout({ logoutParams: { returnTo: window.location.origin } })}
              >
                Sign Out
              </button>
            </>
          ) : (
            <button
              type="button"
              className="hi-bracket text-neutral-900 dark:text-neutral-100"
              onClick={() => loginWithRedirect()}
            >
              Sign In
            </button>
          )}
        </div>

        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetTrigger asChild className="md:hidden absolute right-4 top-4">
            <Button variant="ghost" size="icon" className="text-neutral-900 hover:bg-black/[0.04] dark:text-neutral-100">
              <Menu className="h-6 w-6" />
            </Button>
          </SheetTrigger>
          <SheetContent side="right" className="bg-[#f5f5f5] dark:bg-neutral-950">
            <SheetHeader>
              <SheetTitle className="text-neutral-900 dark:text-neutral-100">Menu</SheetTitle>
            </SheetHeader>
          </SheetContent>
        </Sheet>
      </header>
    );
  }

  return (
    <header
      className={cn(
        "flex justify-around items-center py-[18px] bg-[#051415] relative z-10",
        className,
      )}
    >
      {/* Header Left */}
      <div className="flex items-center gap-8 lg:gap-12">
        <Link to="/" className="flex items-center gap-2.5 text-lg lg:text-xl font-semibold text-white hover:text-[#a0dae4] transition-colors shrink-0">
          <svg className="w-6 lg:w-7 h-6 lg:h-7" viewBox="0 0 28 28" fill="none">
            <rect x="3" y="6" width="22" height="4" rx="1" fill="#a0dae4" />
            <rect x="3" y="12" width="22" height="4" rx="1" fill="#a0dae4" opacity="0.7" />
            <rect x="3" y="18" width="22" height="4" rx="1" fill="#a0dae4" opacity="0.4" />
          </svg>
          FlightStrips
        </Link>

        {/* Desktop Navigation */}
        <nav className="hidden lg:flex gap-8">
          {navLinks.map((link) => (
            <a
              key={link.label}
              href={link.href}
              className="text-sm font-normal text-white hover:text-[#a0dae4] transition-colors flex items-center gap-1"
            >
              {link.label}
            </a>
          ))}
        </nav>
      </div>

      {/* Header Right */}
      <div className="hidden lg:flex gap-5 items-center">
        {isAuthenticated ? (
          <>
            <Link to="/app" className="bg-transparent text-white px-7 py-[11px] border border-[#3a4a4a] rounded-full text-sm font-medium hover:border-[#a0dae4] hover:text-[#a0dae4] transition-colors">
              Open App
            </Link>
            <button
              onClick={() => logout({ logoutParams: { returnTo: window.location.origin } })}
              className="bg-[#a0dae4] text-[#051415] px-[22px] py-[9px] rounded-full text-sm font-medium hover:bg-[#b8e3ec] transition-colors"
            >
              Sign Out
            </button>
          </>
        ) : (
          <>
            <button
              onClick={() => loginWithRedirect()}
              className="bg-transparent text-white px-7 py-[11px] border border-[#3a4a4a] rounded-full text-sm font-medium hover:border-[#a0dae4] hover:text-[#a0dae4] transition-colors"
            >
              Log in
            </button>
            <a
              href="https://docs.flightstrips.dk"
              className="bg-[#a0dae4] text-[#051415] px-[22px] py-[9px] rounded-full text-sm font-medium hover:bg-[#b8e3ec] transition-colors inline-block"
            >
              Get started
            </a>
          </>
        )}
      </div>

      {/* Mobile Menu */}
      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetTrigger asChild className="lg:hidden">
          <Button variant="ghost" size="icon" className="text-white hover:bg-white/10 shrink-0">
            <Menu className="h-6 w-6" />
          </Button>
        </SheetTrigger>
        <SheetContent side="right" className="bg-[#051415] border-l border-[#233434] p-8 w-full sm:w-96">
          <SheetHeader className="mb-8">
            <SheetTitle className="text-white text-left text-2xl">Menu</SheetTitle>
          </SheetHeader>
          <div className="flex flex-col gap-6">
            {/* Navigation Links */}
            {navLinks.map((link) => (
              <a
                key={link.label}
                href={link.href}
                className="text-lg text-white hover:text-[#a0dae4] transition-colors font-medium"
                onClick={() => setMobileOpen(false)}
              >
                {link.label}
              </a>
            ))}

            {/* Divider */}
            <div className="border-t border-[#233434] pt-6" />

            {/* Auth Buttons */}
            <div className="flex flex-col gap-3">
              {isAuthenticated ? (
                <>
                  <Link
                    to="/app"
                    onClick={() => setMobileOpen(false)}
                    className="w-full text-center text-base text-white border border-[#233434] rounded-full py-3 hover:border-[#a0dae4] hover:text-[#a0dae4] transition-colors font-medium"
                  >
                    Open App
                  </Link>
                  <button
                    onClick={() => {
                      logout({ logoutParams: { returnTo: window.location.origin } });
                      setMobileOpen(false);
                    }}
                    className="w-full text-center text-base bg-[#a0dae4] text-[#051415] rounded-full py-3 hover:bg-[#b8e3ec] transition-colors font-medium"
                  >
                    Sign Out
                  </button>
                </>
              ) : (
                <>
                  <button
                    onClick={() => {
                      loginWithRedirect();
                      setMobileOpen(false);
                    }}
                    className="w-full text-center text-base text-white border border-[#233434] rounded-full py-3 hover:border-[#a0dae4] hover:text-[#a0dae4] transition-colors font-medium"
                  >
                    Log in
                  </button>
                  <a
                    href="https://docs.flightstrips.dk"
                    onClick={() => setMobileOpen(false)}
                    className="w-full text-center text-base bg-[#a0dae4] text-[#051415] rounded-full py-3 hover:bg-[#b8e3ec] transition-colors font-medium"
                  >
                    Get started
                  </a>
                </>
              )}
            </div>
          </div>
        </SheetContent>
      </Sheet>
    </header>
  );
}
