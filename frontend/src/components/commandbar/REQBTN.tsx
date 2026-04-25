import { useEffect } from "react";
import { useSetTagRequestArmed, useStrips, useTagRequestArmed } from "@/store/store-hooks";
import { CLS_CMDBTN } from "@/components/strip/shared";

export default function REQBTN() {
  const strips = useStrips();
  const tagRequestArmed = useTagRequestArmed();
  const setTagRequestArmed = useSetTagRequestArmed();
  const canReq = strips.length > 0;

  useEffect(() => {
    if (!canReq && tagRequestArmed) {
      setTagRequestArmed(false);
    }
  }, [canReq, setTagRequestArmed, tagRequestArmed]);

  return (
    <button
      disabled={!canReq}
      className={`${CLS_CMDBTN} ${tagRequestArmed ? "!bg-[#1BFF16] !text-black" : "bg-bay-btn text-white"} ${!canReq ? "opacity-50 cursor-not-allowed" : ""}`}
      onClick={() => canReq && setTagRequestArmed(!tagRequestArmed)}
    >
      REQ
    </button>
  );
}
