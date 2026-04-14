import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import {BrowserRouter, Routes, Route} from "react-router";
import './index.css'
import Home from "@/pages/home";
import About from "@/pages/about";
import FaqPage from "@/pages/faq";
import Privacy from "@/pages/privacy";
import DataHandling from "@/pages/data-handling";
import Contact from "@/pages/contact";
import {Auth0ProviderWithNavigate} from "@/providers/auth-provider";
import Profile from "@/pages/profile";
import Layout from "@/pages/layout";
import Dashboard from "@/pages/dashboard";
import AppPage from "@/pages/app";
import PluginAuthComplete from "@/pages/plugin-auth-complete";
import { withAuthenticationRequired } from '@auth0/auth0-react';
import DocsRouter from "@/pages/docs/DocsRouter";
import { ThemeSync } from "@/components/public/ThemeSync";
import { registerSW } from "virtual:pwa-register";

registerSW({ immediate: true });

const MyProtectedComponent = withAuthenticationRequired(Layout);

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <ThemeSync />
      <Auth0ProviderWithNavigate>
        <Routes>
          <Route path="/" element={<Home/>}/>
          <Route path="/about" element={<About/>}/>
          <Route path="/faq" element={<FaqPage/>}/>
          <Route path="/privacy" element={<Privacy/>}/>
          <Route path="/data-handling" element={<DataHandling/>}/>
          <Route path="/contact" element={<Contact/>}/>
          <Route path="/app" element={<AppPage />}/>
          <Route path="/plugin-auth-complete" element={<PluginAuthComplete/>}/>
          <Route element={<MyProtectedComponent/>}>
            <Route index path="/dashboard" element={<Dashboard/>}/>
            <Route path="/dashboard/profile" element={<Profile/>}/>
            <Route path="/dashboard/docs" element={<DocsRouter />}/>
          </Route>
          <Route path="*" element={<div>404 Not Found</div>}/>
        </Routes>
      </Auth0ProviderWithNavigate>
    </BrowserRouter>
  </StrictMode>,
)
