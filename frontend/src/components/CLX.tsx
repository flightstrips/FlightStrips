import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  useDisclosure,
  Input,
  Textarea,
} from '@nextui-org/react'
import Flightstrip from '../data/interfaces/flightstrip'
import { SIDButton } from './CLX/SIDButton'

export default function CLX(props: {
  destinationICAO: string
  stand: string | null
  Flightstrip: Flightstrip
}) {
  const { isOpen, onOpen, onOpenChange } = useDisclosure()

  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        className="bg-[#BEF5EF] w-[20%] border-r-1 border-l-1 border-t-2 border-b-2 border-[#85B4AF] pl-4 pr-4 h-full flex flex-col  items-center justify-center text-center"
      >
        <span className="font-bold">{props.destinationICAO}</span>
        <span className="font-bold">{props.stand}</span>
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
                        value={props.Flightstrip.destinationICAO}
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
                        <p className="pl-1 pr-1 pt-1 -mt-1 pb-1 text-sm">Route</p>
                        <SIDButton SID="SIMEG 8C"></SIDButton>
                      </div>

                      <Input
                        label="SSR"
                        placeholder=" "
                        labelPlacement="outside"
                        radius="none"
                        className="border-1 border-black w-20"
                        value="6532"
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
                      <Input
                        label="RWY"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-28 ml-6"
                        value="22R"
                      />
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
                        value={props.Flightstrip.actype}
                      />
                      <Input
                        label="FL"
                        placeholder=" "
                        labelPlacement="outside"
                        disabled
                        radius="none"
                        className="border-1 border-black w-16"
                        value="360"
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
                    <div className="flex w-4/5 justify-center mt-4 flex-col">
                      <p className="p-1">Route</p>
                      <Textarea
                        disabled
                        radius="none"
                        className="border-1 border-black w-[32rem] text-center"
                        value="NEXEN T503 GIMRU DCT MICOS DCT RIMET/N0481F390 T157 ODIPI/N0454F210 T157 KERAX KERAX4D N0454F210 T157 KERAX KERAX4DN0454F210 T157 KERAX KERAX4D"
                      />
                    </div>
                    <div className="flex">
                      <Input />
                    </div>
                    <div className="flex">
                      <Input />
                    </div>
                    <div className="flex">
                      <Input />
                      <Input />
                      <Input />
                      <Input />
                      <Input />
                      <Input />
                    </div>
                  </div>
                </fieldset>
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
                    onPress={onClose}
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
}
