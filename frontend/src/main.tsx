import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Routes, Route } from "react-router";
import './index.css'

import Home from "./home.tsx";
import About from "@/about.tsx";
import EKCHDEL from "@/airport/ekch/CLX.tsx";

createRoot(document.getElementById('root')!).render(
  <StrictMode>
      <BrowserRouter>
          <Routes>
              <Route path="/" element={<Home />} />
              <Route path="/about" element={<About />} />
              <Route path="/ekch/clerancedelivery" element={<EKCHDEL />} />
          </Routes>
      </BrowserRouter>
  </StrictMode>,
)
