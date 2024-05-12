import { Select, SelectItem } from '@nextui-org/react'
import {
  useControllerStore,
  useStateStore,
} from '../providers/RootStoreContext.ts'
import { observer } from 'mobx-react'
import { useNavigate } from 'react-router-dom'
import { useEffect } from 'react'

const Startup = observer(() => {
  const stateStore = useStateStore()
  const controllerStore = useControllerStore()
  const navigate = useNavigate()

  useEffect(() => {
    if (stateStore.isReady) {
      navigate(stateStore.view)
    }

    stateStore.loadSessions()
  }, [navigate, stateStore, stateStore.isReady, stateStore.view])

  return (
    <>
      <div className="h-screen w-screen bg-black bg-opacity-50 z-10 absolute flex justify-center items-center flex-col text-white mb-2">
        <svg width="265" height="265" viewBox="0 0 551 640" fill="white">
          <path d="M549.857 280.274L550.36 277.8H547.833H426.056L456.247 128.697L456.747 126.222H454.224H296.91V2.06452V0H294.845H255.513H253.449V2.06452V126.222H96.1366H93.6121L94.113 128.697L124.306 277.8H2.52445H0L0.501017 280.274L39.158 471.184L39.4929 472.836H41.1813H198.97V637.935V640H201.034H240.367H242.431V637.935V472.836H307.927V637.935V640H309.991H349.323H351.388V637.935V472.836H509.177H510.866L511.201 471.184L549.857 280.274ZM296.91 321.261H375.441L364.494 429.378H296.91V321.261ZM253.449 429.378H185.864L174.918 321.261H253.449V429.378ZM146.755 169.683H403.604L381.712 277.8H168.647L146.755 169.683ZM75.0369 429.378L53.1443 321.261H131.235L142.181 429.378H75.0369ZM497.216 321.261L475.323 429.378H408.177L419.123 321.261H497.216Z" />
        </svg>
        <div className="w-1/3">
          <h1 className="text-center text-4xl font-semibold mt-4">
            Flightstrips
          </h1>
        </div>
      </div>
      <div className="absolute bottom-48 left-1/2 transform -translate-x-1/2 z-20 w-1/4">
        <Select
          label="Select session"
          className="mb-2"
          onChange={(e) => stateStore.setSession(e.target.value)}
        >
          {stateStore.availableSessions.map((session) => (
            <SelectItem key={session.name} value={session.name}>
              {session.name}
            </SelectItem>
          ))}
        </Select>

        <Select
          label="Select controller"
          onChange={(e) => stateStore.setController(e.target.value)}
        >
          {controllerStore.controllers.map((controller) => (
            <SelectItem key={controller.callsign} value={controller.callsign}>
              {controller.callsign}
            </SelectItem>
          ))}
        </Select>
      </div>

      <div className="z-0 absolute h-[110vh] w-[110vw] aspect-auto w-screen">
        <img
          src="images/startup.png"
          className="absolute -top-[5vh] h-[110vh] w-[110vw] z-0 blur-sm"
        />
      </div>
    </>
  )
})

export default Startup
