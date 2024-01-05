import React from "react";

export const Header = (props) => {
  return (
    <div className={props.TypeMSG ? 'bg-[#285a5c]' : 'bg-[#393939]'}>
    <div className="w-full h-10 text-white text-xl flex items-center pl-2 pr-2 justify-between font-semibold">
      {props.headerName}
      <div className="flex ">
        {props.buttons}
      </div>
    </div>
    </div>

  );
};
