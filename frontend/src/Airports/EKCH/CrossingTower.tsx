import BayHeader from '../../components/BayHeader'
import { ControllerMessages } from '../../components/ControllerMessages'
import { CommandBar } from '../../components/commandbar'

function CrossingTower() {
  return (
    <div className="bg-background-grey h-screen w-screen flex gap-2 justify-center">
      <div className="w-full bg-bay-grey">
        <div className="h-2/6">
          <BayHeader title="FINAL" />
        </div>
        <div className="h-1/6">
          <BayHeader title="RWY ARR" />
        </div>
        <div className="h-2/6">
          <BayHeader title="RWY ARR" />
        </div>
        <div className="h-1/6">
          <BayHeader title="Stand" />
        </div>
      </div>

      <div className="w-full bg-bay-grey">
        <div className="h-1/6">
          <BayHeader title="PUSH BACK" />
        </div>
        <div className="h-2/6">
          <BayHeader title="TWY DEP" />
        </div>
        <div className="h-2/6">
          <BayHeader title="RWY DEP" />
        </div>
        <div className="h-1/6">
          <BayHeader title="AIRBORNE" />
        </div>
      </div>

      <div className="w-full bg-bay-grey flex flex-col">
        <div className="h-2/3">
          <BayHeader title="CONTROL ZONE" />
        </div>
        <div className="h-2/6">
          <BayHeader
            title="MESSAGES"
            message
            buttons={<ControllerMessages />}
          />
        </div>
      </div>

      <div className="w-full bg-bay-grey">
        <div className="h-4/5">
          <BayHeader title="CLR DEL" />
        </div>
        <div className="h-1/5">
          <BayHeader title="DE-ICE" />
        </div>
      </div>

      <CommandBar />
    </div>
  )
}

export default CrossingTower
