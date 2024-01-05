import React from "react";

export const HalfStrip = (props) => {
  return (
    <div className="w-fit h-10 bg-[#d9d9d9] flex gap-1 justify-center items-center font-bold pl-1 pr-1 m-1">
        <div className="bg-[#bfbfbf] p-1">OB</div>
        <div className="bg-[#bfbfbf] p-1 pr-16">RYR1EB</div>
        <div className="bg-[#bfbfbf] p-1 font-normal pl-4 pr-4">B738</div>
        <div className="bg-[#bfbfbf] p-1 pl-2 pr-2">22R</div>
        <div className="bg-[#bfbfbf] p-1 pl-4 pr-4">A</div>
        <div className="bg-[#bfbfbf] p-1 w-16 font-normal">4</div>
        <div className="bg-[#bfbfbf] p-1 pl-8 pr-8">F7</div>
    </div>
  )
}