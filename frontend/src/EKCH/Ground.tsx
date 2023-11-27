import BayHeader from '../components/BayHeader'
import { CommandBar } from '../components/commandbar'

function Ground() {
  return (
    <div className="bg-background-grey h-screen w-screen flex gap-2 justify-center">
      <div className="w-full bg-bay-grey flex flex-col gap-8">
        <div className="h-1/4">
          <BayHeader title="MESSAGES" msg />
        </div>
        <div className="h-1/3">
          <BayHeader title="RWY ARR" />
        </div>
        <div className="h-1/3">
          <BayHeader title="RWY ARR" />
        </div>
        <div className="h-64">
          <BayHeader title="STAND" />
        </div>
      </div>
      <div className="w-full bg-bay-grey">
        <div className="h-3/5">
          <BayHeader title="TWY DEP" />
        </div>
        <div className="h-1/3">
          <BayHeader title="TWY ARR" />
        </div>
      </div>
      <div className="w-full bg-bay-grey flex flex-col">
        <div className="h-3/6">
          <BayHeader title="STARTUP" />
        </div>
        <div>
          <BayHeader title="PUSH BACK" />
        </div>
      </div>
      <div className="w-full bg-bay-grey">
        <div className="h-4/5">
          <BayHeader title="CLR DEL" />
        </div>
        <div>
          <BayHeader title="DE-ICE" />
        </div>
      </div>

      <CommandBar />
    </div>
  )
}

export default Ground
