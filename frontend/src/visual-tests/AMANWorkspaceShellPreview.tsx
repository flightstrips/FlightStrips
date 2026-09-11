import {createRoot} from "react-dom/client";

import {AMANWorkspaceShell} from "@/components/aman/AMANWorkspaceShell";
import "@/index.css";

createRoot(document.getElementById("root")!).render(
  <AMANWorkspaceShell
    maestro={<div className="grid h-full place-items-center border border-[#777] bg-[#555355] font-display text-2xl font-bold">MAESTRO</div>}
    tmt={<div className="grid h-full place-items-center border border-[#777] bg-[#555355] font-display text-2xl font-bold">TMT</div>}
  />,
);
