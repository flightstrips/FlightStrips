import { Image } from '@nextui-org/react'
import { Input } from '@nextui-org/react'

function SettingsPage() {
  return (
    <div className="bg-white text-black w-screen h-screen flex">
      <div className="w-1/3 h-full bg-slate-700 text-white flex flex-col justify-center items-center">
        <div className="flex flex-row items-center p-4">
          <svg
            width="75"
            height="75"
            viewBox="0 0 551 640"
            fill=""
            xmlns="http://www.w3.org/2000/svg"
            className="fill-white"
          >
            <path d="M549.857 280.274L550.36 277.8H547.833H426.056L456.247 128.697L456.747 126.222H454.224H296.91V2.06452V0H294.845H255.513H253.449V2.06452V126.222H96.1366H93.6121L94.113 128.697L124.306 277.8H2.52445H0L0.501017 280.274L39.158 471.184L39.4929 472.836H41.1813H198.97V637.935V640H201.034H240.367H242.431V637.935V472.836H307.927V637.935V640H309.991H349.323H351.388V637.935V472.836H509.177H510.866L511.201 471.184L549.857 280.274ZM296.91 321.261H375.441L364.494 429.378H296.91V321.261ZM253.449 429.378H185.864L174.918 321.261H253.449V429.378ZM146.755 169.683H403.604L381.712 277.8H168.647L146.755 169.683ZM75.0369 429.378L53.1443 321.261H131.235L142.181 429.378H75.0369ZM497.216 321.261L475.323 429.378H408.177L419.123 321.261H497.216Z" />
          </svg>
          <h1 className="text-4xl font-semibold">FlightStrips</h1>
        </div>
        <h2>Version 0.0.1a</h2>
      </div>
      <div className="w-2/3 h-full flex justify-center">
        <form>
          <div className="flex w-full flex-wrap md:flex-nowrap gap-4">
            <Input
              type="text"
              label="Log directory"
              labelPlacement="outside"
              placeholder="C:\users\vatsca\AppData\FlightStrips\log"
            />
          </div>
        </form>
      </div>
    </div>
  )
}

export default SettingsPage
