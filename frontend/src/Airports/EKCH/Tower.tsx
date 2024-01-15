import BayHeader from '../../components/BayHeader'
import { NewVFR } from '../../components/Buttons/NewVFR'
import { CommandBar } from '../../components/commandbar'

function Tower() {
  return (
    <div className="bg-background-grey h-screen w-screen flex gap-2 justify-center">
      <div className="w-full bg-bay-grey">
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

      <div className="w-full bg-bay-grey">
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

      <div className="w-full bg-bay-grey flex flex-col">
        <div className="h-2/5">
          <BayHeader title="CONTROL ZONE" buttons={<NewVFR />} />
        </div>
        <div className="h-1/4">
          <BayHeader title="PUSH BACK" />
        </div>
        <div className="h-2/6">
          <BayHeader title="MESSAGES" msg />
        </div>
      </div>

      <div className="w-full bg-bay-grey">
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

export default Tower
