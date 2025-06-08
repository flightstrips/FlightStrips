import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Routes, Route, Navigate  } from "react-router";
import './index.css'

import Home from "./home.tsx";
import About from "@/about.tsx";
import EKCHDEL from "@/airport/ekch/CLX.tsx";
import Layout from './airport/Layout.tsx';
import Auth from "@/auth.tsx";

createRoot(document.getElementById('root')!).render(
  <StrictMode>
      <BrowserRouter>
          <Routes>
              <Route path="/" element={<Home />} />
              <Route path="/about" element={<About />} />
              <Route path="/authentication" element={<Auth/>} />
                <Route element={<Layout />}>
                  <Route path="EKCH/CLX" element={<EKCHDEL />} />
                  <Route path="EKCH/AAAD" element={<EKCHDEL />} />
                  <Route path="EKCH/GEGW" element={<EKCHDEL />} />
                  <Route path="EKCH/TWTE" element={<EKCHDEL />} />
                  <Route path="*" element={<Navigate to="/" replace />} />
                </Route>
              <Route path="ekbi" element={<div>Billund not implemented</div>} />
              <Route path="ekyt" element={<div>Aalborg not implemented</div>} />
              <Route path="*" element={<div>404 Not Found</div>} />
          </Routes>
      </BrowserRouter>
  </StrictMode>,
)
