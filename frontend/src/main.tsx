import React from 'react'
import ReactDOM from 'react-dom/client'
//import App from './App.tsx'
import Del from './views/Del.tsx'
import './index.css'

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <Del />
  </React.StrictMode>,
)

postMessage({ payload: 'removeLoading' }, '*')
