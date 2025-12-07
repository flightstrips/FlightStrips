import { startTransition, StrictMode } from "react";
import { hydrateRoot } from "react-dom/client";
import { HydratedRouter } from "react-router/dom";
import Auth0ProviderWithNavigate from "./context/auth-provider";

startTransition(() => {
  hydrateRoot(
    document,
    <StrictMode>
      <Auth0ProviderWithNavigate>
        <HydratedRouter />
      </Auth0ProviderWithNavigate>
    </StrictMode>
  );
});
