import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import {BrowserRouter, Routes, Route} from "react-router";
import './index.css'
import ChartFoxCallbackPage from "@/pages/chartfox-callback";
import FlightStripsRoutes from "@/routes/FlightStripsRoutes";
import { ThemeSync } from "@/components/public/ThemeSync";
import { startAppUpdateMonitoring } from "@/lib/app-update";

startAppUpdateMonitoring();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <ThemeSync />
      <Routes>
        <Route path="/efb/chartfox/callback" element={<ChartFoxCallbackPage />} />
        <Route path="*" element={<FlightStripsRoutes />} />
      </Routes>
    </BrowserRouter>
  </StrictMode>,
)
