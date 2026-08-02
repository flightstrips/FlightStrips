import { Routes, Route } from 'react-router';
import { withAuthenticationRequired } from '@auth0/auth0-react';
import { Auth0ProviderWithNavigate } from '@/providers/auth-provider';
import Home from '@/pages/home';
import About from '@/pages/about';
import FaqPage from '@/pages/faq';
import Privacy from '@/pages/privacy';
import DataHandling from '@/pages/data-handling';
import Contact from '@/pages/contact';
import Profile from '@/pages/profile';
import Layout from '@/pages/layout';
import Dashboard from '@/pages/dashboard';
import AppPage from '@/pages/app';
import PluginAuthComplete from '@/pages/plugin-auth-complete';
import CdmPage from '@/pages/cdm';
import StandStatusPage from '@/pages/stand-status';
import PilotLayout from '@/pages/pilot-layout';
import PilotFlightPage from '@/pages/pilot-flight';
import EfbLayout from '@/pages/efb-layout';
import EfbPage from '@/pages/efb';
import DocsRouter from '@/pages/docs/DocsRouter';
import TestToolsPage from '@/pages/test-tools';
import AMANReplayPage from '@/pages/aman-replay';

const ProtectedLayout = withAuthenticationRequired(Layout);
const ProtectedCdmPage = withAuthenticationRequired(CdmPage);
const ProtectedStandStatusPage = withAuthenticationRequired(StandStatusPage);
const ProtectedTestToolsPage = withAuthenticationRequired(TestToolsPage);

export default function FlightStripsRoutes() {
  return (
    <Auth0ProviderWithNavigate>
      <Routes>
        <Route path="/" element={<Home/>}/>
        <Route path="/about" element={<About/>}/>
        <Route path="/faq" element={<FaqPage/>}/>
        <Route path="/privacy" element={<Privacy/>}/>
        <Route path="/data-handling" element={<DataHandling/>}/>
        <Route path="/contact" element={<Contact/>}/>
        <Route path="/pilot" element={<PilotLayout />}>
          <Route index element={<PilotFlightPage/>}/>
        </Route>
        <Route path="/efb" element={<EfbLayout />}>
          <Route index element={<EfbPage />} />
        </Route>
        <Route path="/app" element={<AppPage />}/>
        <Route path="/plugin-auth-complete" element={<PluginAuthComplete/>}/>
        <Route element={<ProtectedLayout/>}>
          <Route index path="/dashboard" element={<Dashboard/>}/>
          <Route path="/dashboard/profile" element={<Profile/>}/>
          <Route path="/dashboard/docs" element={<DocsRouter />}/>
        </Route>
        <Route path="/cdm" element={<ProtectedCdmPage />} />
        <Route path="/stand" element={<ProtectedStandStatusPage />} />
        <Route path="/test" element={<ProtectedTestToolsPage />} />
        <Route path="/aman-replay" element={<AMANReplayPage />} />
        <Route path="*" element={<div>404 Not Found</div>}/>
      </Routes>
    </Auth0ProviderWithNavigate>
  );
}
