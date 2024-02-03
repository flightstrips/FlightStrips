import {
  Modal,
  ModalContent,
  ModalBody,
  ModalFooter,
  Button,
  useDisclosure,
  Input,
} from '@nextui-org/react'
import { FlightStrip } from '../../stores/FlightStrip'
import { observer } from 'mobx-react'

export const Pushback = observer((props: { Flightstrip: FlightStrip }) => {
  const { isOpen, onOpen, onOpenChange } = useDisclosure()

  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        size="sm"
        className="h-full flex flex-col items-center justify-center text-sm text-center border-t-2 border-b-2 border-white p-0 m-0 bg-[#bef5ef]"
      >
        <div className="border-1 border-t-2 border-b-2 border-[#85B4AF] border-r-[1-px] h-full w-full">
          <span className="font-bold p-0 -mb-1">
            {props.Flightstrip.destination}
          </span>
          <span className="font-bold p-0 -mt-1">{props.Flightstrip.stand}</span>
        </div>
      </Button>
      <Modal
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        size="5xl"
        radius="none"
        classNames={{
          backdrop: 'bg-[#000]/50 backdrop-opacity-40',
          base: 'border-[#292f46] bg-[#e4e4e4] text-[#a8b0d3] drop-shadow-2xl',
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalBody>
                <span className="abselute top-0 left-0 z-10">J4</span>
                <img
                  src="images/apron_push.jpg"
                  alt=""
                  className="abselute top-0 left-0 z-0"
                />
              </ModalBody>
              <ModalFooter className=" justify-center">
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
                  SEARCH
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  )
})
