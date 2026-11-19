import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import {BrowserRouter, Routes, Route} from "react-router";
import './index.css'
import ChartFoxCallbackPage from "@/pages/chartfox-callback";
import FlightStripsRoutes from "@/routes/FlightStripsRoutes";
import { ThemeSync } from "@/components/public/ThemeSync";
import { AppUpdateMonitoring } from "@/components/AppUpdateMonitoring";
import { initAnalytics } from "@/lib/analytics";
import { AnalyticsConsentBanner } from "@/components/AnalyticsConsentBanner";

initAnalytics();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <AppUpdateMonitoring />
      <ThemeSync />
      <AnalyticsConsentBanner />
      <Routes>
        <Route path="/efb/chartfox/callback" element={<ChartFoxCallbackPage />} />
        <Route path="*" element={<FlightStripsRoutes />} />
      </Routes>
    </BrowserRouter>
  </StrictMode>,
)
