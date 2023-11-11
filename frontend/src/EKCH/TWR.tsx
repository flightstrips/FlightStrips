import React from 'react'
import { BayHeader } from '../components/bayheader'
import { CommandBar } from '../components/commandbar'

function TWR() {
  return (
    <div className="bg-slate-400 h-screen w-screen flex gap-2 justify-center">
      <div className="w-full bg-gray-500">
        <div className="h-2/5">
          <BayHeader title="FINAL" />
        </div>
        <div className="h-1/4">
          <BayHeader title="RWY ARR" />
        </div>
        <div className="h-1/3">
          <BayHeader title="RWY ARR" />
        </div>
      </div>

      <div className="w-full bg-gray-500">
        <div className="h-2/5">
          <BayHeader title="TWY DEP" />
        </div>
        <div className="h-1/4">
          <BayHeader title="RWY ARR" />
        </div>
        <div className="h-1/3">
          <BayHeader title="AIRBORNE" />
        </div>
      </div>

      <div className="w-full bg-gray-500 flex flex-col">
        <div className="h-2/5">
          <BayHeader title="CONTROL ZONE" />
        </div>
        <div className="h-1/4">
          <BayHeader title="PUSH BACK" />
        </div>
        <div className="h-2/6">
          <BayHeader title="MESSAGES" msg />
        </div>
      </div>

      <div className="w-full bg-gray-500">
        <div className="h-3/5">
          <BayHeader title="CLR DEL" />
        </div>
        <div className="h-1/5">
          <BayHeader title="DE-ICE" />
        </div>
        <div className="h-1/5">
          <BayHeader title="STAND" />
        </div>
      </div>

      <CommandBar />
    </div>
  )
}

export default TWR
