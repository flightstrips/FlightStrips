import { useState } from "react";
import { Link } from "react-router";
import { useAuth0 } from "@auth0/auth0-react";
import { Menu, Sun, Moon } from "lucide-react";
import {
  NavigationMenu,
  NavigationMenuList,
  NavigationMenuItem,
  NavigationMenuLink,
  navigationMenuTriggerStyle,
} from "@/components/ui/navigation-menu";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { publicNavLinks } from "./publicNavLinks";
import {
  getStoredPublicTheme,
  setStoredPublicTheme,
  type PublicTheme,
} from "@/lib/public-theme";
import { cn } from "@/lib/utils";

const defaultLinkClassName =
  navigationMenuTriggerStyle() +
  " bg-transparent hover:bg-navy/5 dark:hover:bg-white/10 focus:bg-transparent data-[active]:bg-transparent text-navy dark:text-foreground";

const industrialLinkClassName =
  navigationMenuTriggerStyle() +
  " bg-transparent hover:bg-black/[0.04] dark:hover:bg-white/10 focus:bg-transparent data-[active]:bg-transparent text-neutral-900 dark:text-neutral-100 text-[13px] tracking-tight";

export type PublicNavigationTone = "default" | "industrial";

export type PublicNavigationProps = {
  className?: string;
  linkTone?: PublicNavigationTone;
};

export function PublicNavigation({ className, linkTone = "default" }: PublicNavigationProps) {
  const { isAuthenticated, loginWithRedirect, logout } = useAuth0();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [theme, setTheme] = useState<PublicTheme>(() => getStoredPublicTheme());

  const toggleTheme = () => {
    const next = theme === "light" ? "dark" : "light";
    setStoredPublicTheme(next);
    setTheme(next);
  };

  const linkClassName = linkTone === "industrial" ? industrialLinkClassName : defaultLinkClassName;

  return (
    <nav
      className={cn(
        "fixed top-0 left-0 right-0 z-40 flex items-center justify-between px-6 py-4 pr-14 md:pr-8 sm:px-8 md:grid md:grid-cols-3 md:justify-items-stretch border-b border-navy/10 dark:border-white/10 bg-cream/95 dark:bg-background/95 backdrop-blur-md",
        className,
      )}
    >
      <Link
        to="/"
        className={cn(
          "font-display font-semibold text-xl tracking-tight text-navy dark:text-foreground hover:text-primary dark:hover:text-primary transition-colors md:justify-self-start",
          linkTone === "industrial" && "text-neutral-900 hover:text-[#ff5a1f] dark:text-neutral-100 dark:hover:text-[#ff5a1f]",
        )}
      >
        FlightStrips
      </Link>

      {/* Desktop nav – centered */}
      <div className="hidden md:flex items-center justify-center gap-4">
        <NavigationMenu className="max-w-none">
          <NavigationMenuList className="gap-1">
            {publicNavLinks.map((link) => (
              <NavigationMenuItem key={link.to + link.label}>
                <NavigationMenuLink asChild>
                  {"external" in link && link.external ? (
                    <a
                      href={link.to}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={linkClassName}
                    >
                      {link.label}
                    </a>
                  ) : (
                    <Link to={link.to} className={linkClassName}>
                      {link.label}
                    </Link>
                  )}
                </NavigationMenuLink>
              </NavigationMenuItem>
            ))}
          </NavigationMenuList>
        </NavigationMenu>
      </div>
      <div className="hidden md:flex items-center justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={toggleTheme}
          aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
          className={cn(
            "text-navy dark:text-foreground hover:bg-navy/5 dark:hover:bg-white/10",
            linkTone === "industrial" && "text-neutral-800 hover:bg-black/[0.04] dark:text-neutral-200",
          )}
        >
          {theme === "dark" ? (
            <Sun className="h-5 w-5" />
          ) : (
            <Moon className="h-5 w-5" />
          )}
        </Button>
        {isAuthenticated ? (
          <>
            {linkTone === "industrial" ? (
              <Link to="/app" className="hi-bracket text-neutral-900 dark:text-neutral-100">
                Open App
              </Link>
            ) : (
              <Button asChild variant="outline">
                <Link to="/app">Open App</Link>
              </Button>
            )}
            {linkTone === "industrial" ? (
              <button
                type="button"
                className="hi-bracket text-neutral-900 dark:text-neutral-100"
                onClick={() => logout({ logoutParams: { returnTo: window.location.origin } })}
              >
                Sign Out
              </button>
            ) : (
              <Button variant="default" onClick={() => logout({ logoutParams: { returnTo: window.location.origin } })}>
                Sign Out
              </Button>
            )}
          </>
        ) : linkTone === "industrial" ? (
          <button type="button" className="hi-bracket text-neutral-900 dark:text-neutral-100" onClick={() => loginWithRedirect()}>
            Sign In
          </button>
        ) : (
          <Button variant="default" onClick={() => loginWithRedirect()}>
            Sign In
          </Button>
        )}
      </div>

      {/* Mobile menu */}
      <div className="absolute right-4 top-4 flex md:hidden items-center">
        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label="Open menu"
              className={cn(linkTone === "industrial" && "text-neutral-900 hover:bg-black/[0.04] dark:text-neutral-100")}
            >
              <Menu className="h-6 w-6" />
            </Button>
          </SheetTrigger>
          <SheetContent
            side="right"
            className={cn(
              "w-[min(20rem,85vw)] bg-cream dark:bg-background shadow-none p-6",
              linkTone === "industrial" && "bg-[#f5f5f5] dark:bg-neutral-950",
            )}
          >
            <SheetHeader>
              <SheetTitle
                className={cn(
                  "text-left font-display text-navy dark:text-foreground",
                  linkTone === "industrial" && "text-neutral-900 dark:text-neutral-100",
                )}
              >
                Menu
              </SheetTitle>
            </SheetHeader>
            <div className="flex flex-col gap-1 pt-6">
              {publicNavLinks.map((link) =>
                "external" in link && link.external ? (
                  <a
                    key={link.to + link.label}
                    href={link.to}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={linkClassName + " justify-start"}
                    onClick={() => setMobileOpen(false)}
                  >
                    {link.label}
                  </a>
                ) : (
                  <Link
                    key={link.to + link.label}
                    to={link.to}
                    className={linkClassName + " justify-start"}
                    onClick={() => setMobileOpen(false)}
                  >
                    {link.label}
                  </Link>
                )
              )}
              <div className="mt-4 flex items-center gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => {
                    toggleTheme();
                  }}
                  aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
                  className={cn(
                    "shrink-0 text-navy dark:text-foreground",
                    linkTone === "industrial" && "text-neutral-800 hover:bg-black/[0.04] dark:text-neutral-200",
                  )}
                >
                  {theme === "dark" ? (
                    <Sun className="h-5 w-5" />
                  ) : (
                    <Moon className="h-5 w-5" />
                  )}
                </Button>
                {isAuthenticated ? (
                  linkTone === "industrial" ? (
                    <>
                      <Link to="/app" className="hi-bracket flex-1 text-center text-neutral-900 dark:text-neutral-100" onClick={() => setMobileOpen(false)}>
                        Open App
                      </Link>
                      <button
                        type="button"
                        className="hi-bracket flex-1 text-neutral-900 dark:text-neutral-100"
                        onClick={() => {
                          setMobileOpen(false);
                          logout({ logoutParams: { returnTo: window.location.origin } });
                        }}
                      >
                        Sign Out
                      </button>
                    </>
                  ) : (
                    <>
                      <Button asChild variant="outline" className="flex-1" onClick={() => setMobileOpen(false)}>
                        <Link to="/app">Open App</Link>
                      </Button>
                      <Button variant="default" className="flex-1" onClick={() => { setMobileOpen(false); logout({ logoutParams: { returnTo: window.location.origin } }); }}>
                        Sign Out
                      </Button>
                    </>
                  )
                ) : linkTone === "industrial" ? (
                  <button
                    type="button"
                    className="hi-bracket flex-1 text-neutral-900 dark:text-neutral-100"
                    onClick={() => {
                      setMobileOpen(false);
                      loginWithRedirect();
                    }}
                  >
                    Sign In
                  </button>
                ) : (
                  <Button variant="default" className="flex-1" onClick={() => { setMobileOpen(false); loginWithRedirect(); }}>
                    Sign In
                  </Button>
                )}
              </div>
            </div>
          </SheetContent>
        </Sheet>
      </div>
    </nav>
  );
}
