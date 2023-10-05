import React from "react";
import ReactDOM from "react-dom/client";
import { HashRouter, Routes, Route } from "react-router-dom";
const { ipcRenderer } = window.require('electron');
import "./index.css";

import DEL from "./EKCH/DEL";
import GND from "./EKCH/GND";
import TWR from "./EKCH/TWR";
import CTWR from "./EKCH/CTWR";
import Layout from "./layout";
import FIRCS from "./FIRCS";

const root = ReactDOM.createRoot(
  document.getElementById("root") as HTMLElement
);
root.render(
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
  </React.StrictMode>
);
