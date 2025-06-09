import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import {BrowserRouter, Routes, Route, Navigate} from "react-router";
import './index.css'

import Home from "./home.tsx";
import About from "@/about.tsx";
import EKCHDEL from "@/airport/ekch/CLX.tsx";
import AirportLayout from './airport/Layout.tsx';
import Auth from "@/auth.tsx";

import {Auth0ProviderWithNavigate,} from "@/auth-provider.tsx";
import Profile from './profile.tsx';
import Layout from './layout.tsx';
const MyProtectedComponent = withAuthenticationRequired(Layout);
import Dashboard from './dashboard.tsx';
import { withAuthenticationRequired } from '@auth0/auth0-react';
import DocsRouter from './DocsRouter.tsx';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <Auth0ProviderWithNavigate>
        <Routes>
          <Route path="/" element={<Home/>}/>
          <Route path="/about" element={<About/>}/>
          <Route path="/login" element={<Auth/>}/>
          <Route element={<MyProtectedComponent/>}>
            <Route index path="/dashboard" element={<Dashboard/>}/>
            <Route path="/dashboard/profile" element={<Profile/>}/>
            <Route path="/dashboard/docs" element={<DocsRouter />}/>
          </Route>
          <Route element={<AirportLayout/>}>
            <Route path="EKCH/CLX" element={<EKCHDEL/>}/>
            <Route path="EKCH/AAAD" element={<EKCHDEL/>}/>
            <Route path="EKCH/GEGW" element={<EKCHDEL/>}/>
            <Route path="EKCH/TWTE" element={<EKCHDEL/>}/>
          </Route>
          <Route path="ekbi" element={<div>Billund not implemented</div>}/>
          <Route path="ekyt" element={<div>Aalborg not implemented</div>}/>
          <Route path="*" element={<div>404 Not Found</div>}/>
        </Routes>
      </Auth0ProviderWithNavigate>
    </BrowserRouter>
  </StrictMode>,
)
