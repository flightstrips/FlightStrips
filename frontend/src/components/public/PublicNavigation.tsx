import { useState, useEffect } from "react";
import { Link } from "react-router";
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

const linkClassName =
  navigationMenuTriggerStyle() +
  " bg-transparent hover:bg-navy/5 dark:hover:bg-white/10 focus:bg-transparent data-[active]:bg-transparent text-navy dark:text-foreground";

export function PublicNavigation() {
  const [mobileOpen, setMobileOpen] = useState(false);
  const [theme, setTheme] = useState<PublicTheme>(() => getStoredPublicTheme());

  useEffect(() => {
    setTheme(getStoredPublicTheme());
  }, []);

  const toggleTheme = () => {
    const next = theme === "light" ? "dark" : "light";
    setStoredPublicTheme(next);
    setTheme(next);
  };

  return (
    <nav className="fixed top-0 left-0 right-0 z-40 flex items-center justify-between px-6 py-4 pr-14 md:pr-8 sm:px-8 md:grid md:grid-cols-3 md:justify-items-stretch border-b border-navy/10 dark:border-white/10 bg-cream/95 dark:bg-background/95 backdrop-blur-md">
      <Link
        to="/"
        className="font-display font-semibold text-xl tracking-tight text-navy dark:text-foreground hover:text-primary dark:hover:text-primary transition-colors md:justify-self-start"
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
          className="text-navy dark:text-foreground hover:bg-navy/5 dark:hover:bg-white/10"
        >
          {theme === "dark" ? (
            <Sun className="h-5 w-5" />
          ) : (
            <Moon className="h-5 w-5" />
          )}
        </Button>
        <Button asChild variant="default">
          <Link to="/login">Login</Link>
        </Button>
      </div>

      {/* Mobile menu */}
      <div className="absolute right-4 top-4 flex md:hidden items-center">
        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="Open menu">
              <Menu className="h-6 w-6" />
            </Button>
          </SheetTrigger>
          <SheetContent
            side="right"
            className="w-[min(20rem,85vw)] bg-cream dark:bg-background shadow-none p-6"
          >
            <SheetHeader>
              <SheetTitle className="text-left font-display text-navy dark:text-foreground">
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
                  className="shrink-0 text-navy dark:text-foreground"
                >
                  {theme === "dark" ? (
                    <Sun className="h-5 w-5" />
                  ) : (
                    <Moon className="h-5 w-5" />
                  )}
                </Button>
                <Button asChild variant="default" className="flex-1">
                  <Link to="/login" onClick={() => setMobileOpen(false)}>
                    Login
                  </Link>
                </Button>
              </div>
            </div>
          </SheetContent>
        </Sheet>
      </div>
    </nav>
  );
}
