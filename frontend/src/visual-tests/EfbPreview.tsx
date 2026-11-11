import { Auth0Context, initialContext, type GetTokenSilentlyOptions } from '@auth0/auth0-react';
import { createRoot } from 'react-dom/client';
import EFBPage from '@/pages/efb';
import '@/index.css';

function getToken(options: GetTokenSilentlyOptions & { detailedResponse: true }): Promise<undefined>;
function getToken(options?: GetTokenSilentlyOptions): Promise<string>;
async function getToken(options?: GetTokenSilentlyOptions): Promise<string | undefined> {
  return options?.detailedResponse ? undefined : 'test-token';
}

createRoot(document.getElementById('root')!).render(
  <Auth0Context.Provider value={{ ...initialContext, isAuthenticated: true, isLoading: false, getAccessTokenSilently: getToken }}>
    <EFBPage />
  </Auth0Context.Provider>,
);
