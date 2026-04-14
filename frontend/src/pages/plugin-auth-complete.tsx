import { ArrowRight, CheckCircle2, Home, Monitor } from "lucide-react";
import { useAuth0 } from "@auth0/auth0-react";
import { Link } from "react-router";
import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";

export default function PluginAuthComplete() {
  const { isAuthenticated, loginWithRedirect } = useAuth0();

  return (
    <div className="min-h-screen bg-cream dark:bg-background text-navy dark:text-foreground flex flex-col">
      <PublicNavigation />

      <main className="flex-1 px-6 sm:px-8 pt-28 pb-16 sm:pt-36 sm:pb-24">
        <div className="max-w-4xl mx-auto">
          <div className="mb-6 inline-flex items-center gap-2 rounded-full border border-primary/20 bg-primary/5 px-4 py-2 text-xs font-medium uppercase tracking-[0.2em] text-primary">
            <CheckCircle2 className="h-4 w-4" />
            EuroScope sign-in complete
          </div>

          <h1
            className="font-display font-semibold text-4xl sm:text-5xl md:text-6xl tracking-tight text-navy dark:text-foreground mb-6"
            style={{ letterSpacing: "-0.02em", lineHeight: 1.05 }}
          >
            You are logged in.
          </h1>

          <p className="max-w-2xl text-lg sm:text-xl font-light leading-relaxed text-navy/80 dark:text-foreground/80 mb-10">
            FlightStrips for EuroScope has received your sign-in. You can return to EuroScope now, or stay here and continue on the website.
          </p>

          <div className="flex flex-col sm:flex-row gap-4 mb-12">
            <Button asChild size="lg" className="bg-primary hover:bg-primary/90 text-white dark:text-navy rounded-sm shadow-sm w-fit">
              <Link to="/">
                Back to homepage
                <Home className="ml-2 h-4 w-4" />
              </Link>
            </Button>
            {isAuthenticated ? (
              <Button asChild variant="outline" size="lg" className="border-2 border-primary/50 text-primary hover:bg-primary/10 rounded-sm w-fit">
                <Link to="/app">
                  Open web app
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Link>
              </Button>
            ) : (
              <Button
                variant="outline"
                size="lg"
                className="border-2 border-primary/50 text-primary hover:bg-primary/10 rounded-sm w-fit"
                onClick={() => loginWithRedirect({ appState: { returnTo: "/app" } })}
              >
                Sign in
                <ArrowRight className="ml-2 h-4 w-4" />
              </Button>
            )}
          </div>

          <Card className="border-cream dark:border-border bg-white/80 dark:bg-card/80 shadow-sm">
            <CardContent className="p-8">
              <div className="grid gap-6 md:grid-cols-2">
                <div className="rounded-xl border border-primary/15 bg-primary/5 p-6">
                  <div className="mb-4 inline-flex rounded-lg border border-primary/15 bg-white/80 dark:bg-background/80 p-3">
                    <Monitor className="h-5 w-5 text-primary" />
                  </div>
                  <h2 className="font-display font-semibold text-2xl tracking-tight mb-3">
                    Back in EuroScope
                  </h2>
                  <p className="font-light leading-relaxed text-navy/75 dark:text-foreground/75">
                    You can return to EuroScope now. Sign-in is complete and FlightStrips is ready.
                  </p>
                </div>

                <div className="rounded-xl border border-navy/10 dark:border-border bg-cream/40 dark:bg-background/40 p-6">
                  <h2 className="font-display font-semibold text-2xl tracking-tight mb-3">
                    Continue on the website
                  </h2>
                  <p className="font-light leading-relaxed text-navy/75 dark:text-foreground/75 mb-4">
                    Browse the public site, open the web app, or just leave this tab open for later. Nothing else is required here.
                  </p>
                  <p className="text-sm text-navy/60 dark:text-foreground/60">
                    For simulation use only. Not for real-world operations.
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </main>

      <PublicFooter />
    </div>
  );
}
