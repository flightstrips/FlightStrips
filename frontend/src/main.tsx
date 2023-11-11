import React from 'react'
import ReactDOM from 'react-dom/client'
import { HashRouter, Routes, Route } from 'react-router-dom'
import './index.css'

import DEL from './EKCH/DEL'
import GND from './EKCH/GND'
import TWR from './EKCH/TWR'
import CTWR from './EKCH/CTWR'
import FIRCS from './FIRCS'

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <HashRouter>
      <Routes>
        <Route path="/" element={<FIRCS />}>
          <Route path="ekch/">
            <Route path="del" element={<DEL />} />
            <Route path="gnd" element={<GND />} />
            <Route path="twr" element={<TWR />} />
            <Route path="ctwr" element={<CTWR />} />
          </Route>
        </Route>
      </Routes>
    </HashRouter>
  </React.StrictMode>,
)
