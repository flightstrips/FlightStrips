import { useState } from 'react'

export function CommandBar() {
  const [runways] = useState({ dep: '22R', arr: '22L' })
  const [atis] = useState({
    QNH: 1015,
    letter: 'L',
    winds: '350/35KT',
  })

  return (
    <div className="flex w-full h-14 bg-[#3C3C3C] absolute bottom-0 justify-between">
      <div className="flex items-center">
        <div className="flex bg-[#1BFF16] w-32 h-10  text-black text-xs font-bold text-center ml-2 justify-center items-center">
          DEL - A/D_GND A/C/D_TWR
        </div>
        <p className="font-black text-2xl ml-2 mr-2 text-white">DEP</p>
        <div className="flex bg-[#E4E4E4] w-16 h-10 text-black justify-center items-center font-bold text-xl">
          {runways.dep}
        </div>
        <p className="font-black text-2xl ml-2 mr-2 text-white">ARR</p>
        <div className="flex bg-[#E4E4E4] w-16 h-10 text-black justify-center items-center font-bold text-xl">
          {runways.arr}
        </div>
        <p className="font-black text-2xl ml-2 mr-2 text-white">QNH</p>
        <div className="flex bg-[#212121] w-16 h-10 text-white justify-center items-center font-bold text-xl">
          {atis.QNH}
        </div>
        <button className="flex bg-[#646464] border-[#E4E4E4] border-t-2 border-l-2 w-16 h-10 text-white justify-center items-center font-bold text-xl ml-4">
          ATIS
        </button>
        <div className="flex bg-[#E4E4E4] w-12 h-10 text-black justify-center items-center font-bold text-xl">
          {atis.letter}
        </div>
        <div className="flex bg-[#E4E4E4] w-32 h-10 text-black justify-center items-center font-bold text-xl ml-4">
          {atis.winds}
        </div>
      </div>
      <div className="flex items-center">
        <button className="flex bg-[#646464] border-[#E4E4E4] border-t-2 border-l-2 w-16 h-10 text-white justify-center items-center font-bold text-xl mr-1">
          TRF
        </button>
        <button className="flex bg-[#646464] border-[#E4E4E4] border-t-2 border-l-2 w-16 h-10 text-white justify-center items-center font-bold text-xl mr-1">
          MRK
        </button>
        <button className="flex bg-[#646464] border-[#E4E4E4] border-t-2 border-l-2 w-16 h-10 text-white justify-center items-center font-bold text-xl mr-1">
          REQ
        </button>
        <button className="flex bg-[#646464] border-[#E4E4E4] border-t-2 border-l-2 w-16  h-10 text-white justify-center items-center font-bold text-xl mr-2">
          X
        </button>
        <div className="flex bg-[#E4E4E4] w-28 h-10 text-black justify-center items-center font-bold text-md mr-2">
          19:24:56z
        </div>
      </div>
    </div>
  )
}
