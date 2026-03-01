import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import {BrowserRouter, Routes, Route} from "react-router";
import './index.css'
import Home from "@/pages/home";
import About from "@/pages/about";
import Privacy from "@/pages/privacy";
import DataHandling from "@/pages/data-handling";
import EKCHDEL  from "@/routes/ekch/CLX";
import EKCHAAAD from "@/routes/ekch/AAAD";
import EKCHGEGW from "@/routes/ekch/GEGW";
import EKCHTWTE from "@/routes/ekch/TWTE";
import AirportLayout from "@/routes/Layout";
import Auth from "@/pages/auth";
import {Auth0ProviderWithNavigate} from "@/providers/auth-provider";
import Profile from "@/pages/profile";
import Layout from "@/pages/layout";
import Dashboard from "@/pages/dashboard";
import { withAuthenticationRequired } from '@auth0/auth0-react';
import DocsRouter from "@/pages/docs/DocsRouter";

const MyProtectedComponent = withAuthenticationRequired(Layout);

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <Auth0ProviderWithNavigate>
        <Routes>
          <Route path="/" element={<Home/>}/>
          <Route path="/about" element={<About/>}/>
          <Route path="/privacy" element={<Privacy/>}/>
          <Route path="/data-handling" element={<DataHandling/>}/>
          <Route path="/login" element={<Auth/>}/>
          <Route element={<MyProtectedComponent/>}>
            <Route index path="/dashboard" element={<Dashboard/>}/>
            <Route path="/dashboard/profile" element={<Profile/>}/>
            <Route path="/dashboard/docs" element={<DocsRouter />}/>
          </Route>
          <Route element={<AirportLayout/>}>
            <Route path="EKCH/CLX"  element={<EKCHDEL/>}/>
            <Route path="EKCH/AAAD" element={<EKCHAAAD/>}/>
            <Route path="EKCH/GEGW" element={<EKCHGEGW/>}/>
            <Route path="EKCH/TWTE" element={<EKCHTWTE/>}/>
          </Route>
          <Route path="ekbi" element={<div>Billund not implemented</div>}/>
          <Route path="ekyt" element={<div>Aalborg not implemented</div>}/>
          <Route path="*" element={<div>404 Not Found</div>}/>
        </Routes>
      </Auth0ProviderWithNavigate>
    </BrowserRouter>
  </StrictMode>,
)
