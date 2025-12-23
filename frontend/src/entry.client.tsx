import { startTransition, StrictMode } from "react";
import { hydrateRoot } from "react-dom/client";
import { HydratedRouter } from "react-router/dom";
import Auth0ProviderWithNavigate from "./components/providers/auth-provider";
import { ThemeProvider } from "./components/providers/theme-provider";

startTransition(() => {
  hydrateRoot(
    document,
    <StrictMode>
      <Auth0ProviderWithNavigate>
        <ThemeProvider defaultTheme="light" storageKey="fs-ui-theme">
          <HydratedRouter />
        </ThemeProvider>
      </Auth0ProviderWithNavigate>
    </StrictMode>
  );
});
