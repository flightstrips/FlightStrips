import * as React from "react";
import { Route, Routes } from "react-router";

//Views
import { Delivery } from "./views/Delivery";
import { Apron } from "./views/Apron";
import { ADTower } from "./views/ADTower";
import { CTower } from "./views/CTower";

function App() {
  return (
    <Routes>
      <Route path="/" element={<Delivery />}>
        <Route path="/apron" element={<Delivery />}/>
        <Route path="/apron" element={<Apron />}/>
        <Route path="/adtower" element={<ADTower />}/>
        <Route path="/ctower" element={<CTower />}/>
      </Route>
    </Routes>
  );
}

export default App;
