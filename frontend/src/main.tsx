import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Del from './views/del/Del'
import './index.css'
import Selection from './views/selection/Selection'
import { Layout } from './Layout'
import { RootStoreProvider } from './providers/RootStoreProvider'

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <RootStoreProvider>
      <BrowserRouter>
        <Routes>
          <Route index element={<Selection />} />
          <Route path="/ekch" element={<Layout />}>
            <Route path="del" element={<Del />} />
            <Route path="ae" element={<h1>AE</h1>} />
            <Route path="aw" element={<h1>AW</h1>} />
          </Route>
        </Routes>
      </BrowserRouter>
    </RootStoreProvider>
  </React.StrictMode>,
)

postMessage({ payload: 'removeLoading' }, '*')
