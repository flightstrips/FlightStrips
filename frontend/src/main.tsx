import React from 'react'
import ReactDOM from 'react-dom/client'
import { HashRouter, Routes, Route } from 'react-router-dom'
import { NextUIProvider } from '@nextui-org/react'
import './index.css'

import DEL from './EKCH/Delivery.tsx'
import GND from './EKCH/Ground'
import TWR from './EKCH/Tower'
import CTWR from './EKCH/CrossingTower'

import { RootStoreProvider } from './providers/RootStoreProvider.tsx'

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
