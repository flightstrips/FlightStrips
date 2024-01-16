import {
  Modal,
  ModalContent,
  ModalBody,
  ModalFooter,
  Button,
  useDisclosure,
  Input,
} from '@nextui-org/react'

export const MemoryAid = () => {
  const { isOpen, onOpen, onOpenChange } = useDisclosure()

  function PreDefMSG(props: { text: string }) {
    return (
      <Button
        radius="none"
        className="w-full mt-1 mb-1 font-semibold drop-shadow-md text-lg justify-items-start text-left"
      >
        {props.text}
      </Button>
    )
  }

  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        size="sm"
        className="bg-[#004fd6] border-white border-2 mr-1 text-white text-md"
      >
        MEM AID
      </Button>
      <Modal
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        size="5xl"
        radius="none"
        classNames={{
          backdrop: 'bg-[#000]/75 backdrop-opacity-40',
          base: 'border-[#292f46] bg-[#e4e4e4] text-[#a8b0d3] drop-shadow-2xl',
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalBody className="border-1 border-black m-4">
                <div className="flex justify-center items-center flex-col">
                  <div className="w-5/6 mb-8">
                    <p className="text-center text-black pl-4 pt-4 pr-4">
                      FREE TEXT
                    </p>
                    <Input
                      placeholder="Text can be written down here"
                      radius="none"
                      size="lg"
                      classNames={{ input: 'text-2xl text-center' }}
                    />
                  </div>
                  <div className="flex justify-evenly">
                    <div className="w-2/3">
                      <PreDefMSG text="ALL DEPARTURES ON RWY HDG" />
                      <PreDefMSG text="3 MINUTES SEPARATION" />
                      <PreDefMSG text="NO RIGHT TURN AFTER DEPARTURE" />
                      <PreDefMSG text="STOP CLIMB AT 3000'" />
                      <PreDefMSG text="STOP CLIMB AT 4000'" />
                      <PreDefMSG text="OUTBOUND ON K1" />
                      <PreDefMSG text="OUTBOUND ON K3" />
                      <PreDefMSG text="OUTBOUND ON K3" />
                      <PreDefMSG text="OLD AIRAC SID" />
                    </div>
                  </div>
                </div>
                <div className="w-full flex justify-center">
                  <Button
                    radius="none"
                    className="bg-[#3f3f3f] text-white text-2xl p-2 ml-4 mr-4"
                    onPress={onClose}
                  >
                    ERASE
                  </Button>
                  <Button
                    radius="none"
                    className="bg-[#3f3f3f] text-white text-2xl p-2 ml-4 mr-4"
                    onPress={onClose}
                  >
                    OK
                  </Button>
                </div>
              </ModalBody>
              <ModalFooter className="justify-evenly"></ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  )
}
