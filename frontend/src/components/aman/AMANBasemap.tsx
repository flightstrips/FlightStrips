import {TileLayer} from "react-leaflet";

import "./aman-basemap.css";

export function AMANBasemap() {
  return <TileLayer
    attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
    className="aman-basemap"
    keepBuffer={2}
    maxZoom={19}
    url="https://tile.openstreetmap.org/{z}/{x}/{y}.png"
  />;
}
