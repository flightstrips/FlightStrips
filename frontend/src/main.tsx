import React from 'react'
import ReactDOM from 'react-dom/client'
import { HashRouter, Routes, Route } from 'react-router-dom'
import { NextUIProvider } from '@nextui-org/react'
import './index.css'

import DEL from './views/Airports/EKCH/Delivery.tsx'
import GND from './views/Airports/EKCH/Ground.tsx'
import TWR from './views/Airports/EKCH/Tower.tsx'
import CTWR from './views/Airports/EKCH/CrossingTower.tsx'

import { RootStoreProvider } from './providers/RootStoreProvider.tsx'
import Startup from './views/Startup.tsx'

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <RootStoreProvider>
      <NextUIProvider>
        <HashRouter>
          <Routes>
            <Route path="/" element={<Startup />} />
            <Route path="/ekch/del" element={<DEL />} />
            <Route path="/ekch/gnd" element={<GND />} />
            <Route path="/ekch/twr" element={<TWR />} />
            <Route path="/ekch/ctwr" element={<CTWR />} />
          </Routes>
        </HashRouter>
      </NextUIProvider>
    </RootStoreProvider>
  </React.StrictMode>,
)

postMessage({ payload: 'removeLoading' }, '*')
