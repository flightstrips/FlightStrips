import React from 'react'
import ReactDOM from 'react-dom/client'
import { HashRouter, Routes, Route } from 'react-router-dom'
import { NextUIProvider } from '@nextui-org/react'
import './index.css'

import DEL from './Airports/EKCH/Delivery.tsx'
import GND from './Airports/EKCH/Ground.tsx'
import TWR from './Airports/EKCH/Tower.tsx'
import CTWR from './Airports/EKCH/CrossingTower.tsx'

import { RootStoreProvider } from './providers/RootStoreProvider.tsx'
import Startup from './views/selection/Startup.tsx'

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <RootStoreProvider>
      <NextUIProvider>
        <HashRouter>
          <Routes>
            <Route path="/" element={<DEL />} />
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
