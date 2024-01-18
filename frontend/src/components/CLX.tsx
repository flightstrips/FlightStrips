import {
  Modal,
  ModalContent,
  ModalBody,
  Button,
  useDisclosure,
  Input,
  Textarea,
} from '@nextui-org/react'
import { SIDButton } from './CLX/SIDButton'
import { FlightStrip } from '../stores/FlightStrip'
import { observer } from 'mobx-react'
import { RunwayButton } from './CLX/Rynway'

// eslint-disable-next-line react-refresh/only-export-components
export const CLX = observer((props: { Flightstrip: FlightStrip }) => {
  const { isOpen, onOpen, onOpenChange } = useDisclosure()

  function IsArrivalAircraft(Departure: string, Destination: string) {
    if (Departure === Destination) {
      return 'true'
    }
  }

  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        className="bg-[#BEF5EF] w-16 border-r-1 border-l-1 border-t-2 border-b-2 border-[#85B4AF] pl-4 pr-4 h-full flex flex-col  items-center justify-center text-center"
      >
        <span className="font-bold p-0 -mb-1">
          {props.Flightstrip.destination}
        </span>
        <span className="font-bold p-0 -mt-1">{props.Flightstrip.stand}</span>
      </Button>
      <Modal
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        size="2xl"
        classNames={{
          backdrop: 'bg-[#000]/50 backdrop-opacity-40 w-screen h-screen z-10',
          base: 'bg-[#d4d4d4] drop-shadow-2xl',
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalBody>
                <fieldset className="border-2 border-black h-fit mt-4 mb-4 flex items-center justify-center ">
                  <legend className="pl-4 pr-4 text-center text-lg">
                    FLIGHT PLAN
                  </legend>
                  <div className="flex flex-col justtify-center items-center">
                    <div className="flex w-4/5 justify-center gap-2">
                      <Input
                        label="C/S"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-32"
                        value={props.Flightstrip.callsign}
                      />
                      <Input
                        label="ADES"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-20"
                        value={props.Flightstrip.destination}
                      />
                      <Input
                        label="RNAV"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-16"
                        value=" "
                      />
                      <div className="flex flex-col">
                        <p className="pl-1 pr-1 pt-1 -mt-1 pb-1 text-sm">SID</p>
                        <SIDButton SID={props.Flightstrip.sid}></SIDButton>
                      </div>

                      <Input
                        label="SSR"
                        placeholder=" "
                        labelPlacement="outside"
                        radius="none"
                        className="border-1 border-black w-20 text-center"
                        value={props.Flightstrip.squawk}
                      />
                      <Input
                        label="CTOT"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-20 disabled:bg-slate-900"
                        value={props.Flightstrip.ctot}
                      />
                    </div>
                    <div className="flex w-4/5 justify-center gap-2 mt-4">
                      <Input
                        label="EOBT"
                        placeholder=" "
                        labelPlacement="outside"
                        radius="none"
                        className="border-1 border-black w-20"
                        value={props.Flightstrip.eobt}
                      />
                      <Input
                        label="TOBT"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-20"
                        value={props.Flightstrip.tsat}
                      />
                      <Input
                        label="TSAT"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-20"
                        value={props.Flightstrip.tsat}
                      />
                      <div className="flex flex-col">
                        <p className="pl-1 pr-1 pt-1 -mt-1 pb-1 text-sm">RWY</p>
                        <RunwayButton Runway={props.Flightstrip.runway} />
                      </div>
                      <Input
                        label="REA"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-16 ml-32"
                        value=""
                      />
                    </div>
                    <div className="flex w-4/5 justify-center gap-2 mt-4">
                      <Input
                        label="TYPE"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-48"
                        value={props.Flightstrip.aircraftType}
                      />
                      <Input
                        label="FL"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-16"
                        value={props.Flightstrip.fl}
                      />
                      <Input
                        label="SPEED"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-16"
                        value="450"
                      />
                      <Input
                        label="STS"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-56"
                        value=" "
                      />
                    </div>
                    <div className="flex w-4/5 justify-center mt-2 flex-col">
                      <p className="p-1">Route</p>
                      <Textarea
                        disabled
                        radius="none"
                        className="border-1 border-black w-[32rem] text-center"
                        value={props.Flightstrip.route}
                      />
                    </div>
                    <div className="flex w-4/5 justify-center flex-col">
                      <p className="p-1">COPANS REMARKS</p>
                      <Textarea
                        disabled
                        radius="none"
                        className="border-1 border-black w-[32rem] text-center"
                        value={props.Flightstrip.remarks}
                      />
                    </div>
                    <div className="flex w-4/5 justify-center flex-col">
                      <div className="flex justify-around mt-2">
                        <Input
                          label="NITOS REMARKS"
                          placeholder=" "
                          labelPlacement="outside"
                          disabled
                          radius="none"
                          className="border-1 border-black w-full mr-2 text-center"
                          value=""
                        />
                        <Input
                          label="IATA TYPE"
                          placeholder=" "
                          labelPlacement="outside"
                          disabled
                          radius="none"
                          className="border-1 border-black w-32"
                          value=" "
                        />
                      </div>
                    </div>
                    <div className="flex w-4/5 justify-center gap-2 mt-4 mb-4">
                      <Input
                        label="CLIMB"
                        placeholder=" "
                        labelPlacement="outside"
                        radius="none"
                        className="border-1 border-black w-full"
                        value="M"
                      />
                      <Input
                        label="HDG"
                        placeholder=" "
                        labelPlacement="outside"
                        radius="none"
                        className="border-1 border-black w-full"
                        value={props.Flightstrip.hdg}
                      />
                      <Input
                        label="ALT"
                        placeholder=" "
                        labelPlacement="outside"
                        radius="none"
                        className="border-1 border-black w-full"
                        value={props.Flightstrip.alt}
                      />
                      <Input
                        label="De-ICE"
                        placeholder=" "
                        labelPlacement="outside"
                        radius="none"
                        className="border-1 border-black w-full"
                        value={props.Flightstrip.deice}
                      />
                      <Input
                        label="REG"
                        placeholder=" "
                        labelPlacement="outside"
                        radius="none"
                        className="border-1 border-black w-full"
                        value={props.Flightstrip.reg}
                      />
                      <Input
                        label="STAND"
                        placeholder=" "
                        labelPlacement="outside"
                        radius="none"
                        className="border-1 border-black w-full"
                        value={props.Flightstrip.stand}
                      />
                    </div>
                  </div>
                </fieldset>
                {IsArrivalAircraft(
                  props.Flightstrip.origin,
                  props.Flightstrip.destination,
                ) ? (
                  <fieldset className="border-2 border-black h-fit mt-4 mb-4 flex items-center justify-center ">
                    <legend className="pl-4 pr-4 text-center text-lg">
                      ARRIVAL
                    </legend>
                    <div className="flex flex-col justtify-center items-center">
                      <div className="flex w-4/5 justify-center gap-2 mb-4">
                        <Input
                          label="ADEP"
                          placeholder=" "
                          labelPlacement="outside"
                          radius="none"
                          className="border-1 border-black w-full"
                          value=""
                          disabled
                        />
                        <Input
                          label="STAR"
                          placeholder=" "
                          labelPlacement="outside"
                          radius="none"
                          className="border-1 border-black w-full"
                          value=""
                          disabled
                        />
                        <div className="flex flex-col">
                          <p className="pl-1 pr-1 pt-1 -mt-1 pb-1 text-sm">
                            RWY
                          </p>
                          <RunwayButton Runway={props.Flightstrip.runway} />
                        </div>
                        <Input
                          label="ETA"
                          placeholder=" "
                          labelPlacement="outside"
                          radius="none"
                          className="border-1 border-black w-full"
                          value=""
                          disabled
                        />
                        <Input
                          label="AOBT"
                          placeholder=" "
                          labelPlacement="outside"
                          radius="none"
                          className="border-1 border-black w-full"
                          value=""
                          disabled
                        />
                      </div>
                    </div>
                  </fieldset>
                ) : (
                  <></>
                )}

                <div className="flex justify-between">
                  <Button
                    radius="none"
                    size="lg"
                    className="text-xl bg-[#3F3F3F] text-white m-4"
                    onPress={onClose}
                  >
                    ESC
                  </Button>
                  <Button
                    radius="none"
                    size="lg"
                    className="text-xl bg-[#3F3F3F] text-white m-4"
                    onPress={() => props.Flightstrip.clear()}
                  >
                    CLD
                  </Button>
                </div>
              </ModalBody>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  )
})
