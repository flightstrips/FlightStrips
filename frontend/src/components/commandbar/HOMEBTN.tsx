import { useState } from "react";
import { useNavigate, useLocation } from "react-router";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogTrigger,
} from "@/components/ui/dialog";

const EKCH_SCOPES = [
  { label: "CLR DEL", path: "/EKCH/CLX" },
  { label: "AA + AD", path: "/EKCH/AAAD" },
  { label: "GE / GW", path: "/EKCH/GEGW" },
  { label: "TW / TE", path: "/EKCH/TWTE" },
];

export default function HOMEBTN() {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();

  const handleSelect = (path: string) => {
    navigate(path);
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <button className="bg-[#646464] text-xl font-bold p-2 border-2">
          <img src="/home.svg" width="39" height="39" alt="home icon" />
        </button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[300px] bg-[#b3b3b3]">
        <div className="border-2 border-black">
          <div className="grid grid-cols-2 gap-2 p-2">
            {EKCH_SCOPES.map((scope) => (
              <Button
                key={scope.path}
                variant="trf"
                className={`font-normal text-base h-fit py-3 ${
                  location.pathname === scope.path ? "ring-2 ring-yellow-400" : ""
                }`}
                onClick={() => handleSelect(scope.path)}
              >
                {scope.label}
              </Button>
            ))}
          </div>
          <DialogFooter className="flex justify-center w-full h-14">
            <Button variant="darkaction" className="w-4/5" onClick={() => setOpen(false)}>
              ESC
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  );
}