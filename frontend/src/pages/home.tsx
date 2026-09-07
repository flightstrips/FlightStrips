import { LandingPage } from "@/components/public/landing/LandingPage";

/**
 * The public landing page.
 *
 * All of it lives in `@/components/public/landing`, which depends on nothing
 * from the application except `useLandingAuth`. That folder is intended to move
 * to a standalone public site; this route is the only thing holding it here.
 */
export default function Home() {
  return <LandingPage />;
}
