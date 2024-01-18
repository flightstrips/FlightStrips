import {
  Modal,
  ModalContent,
  ModalBody,
  Button,
  useDisclosure,
} from '@nextui-org/react'

export function RunwayButton(props: { Runway: string }) {
  const { isOpen, onOpen, onOpenChange } = useDisclosure()
  const Runways = ['04R', '04L', '12', '22R', '22L', '30']
  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        className="border-1 border-black w-full bg-default-100"
      >
        {props.Runway}
      </Button>
      <Modal
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        size="xs"
        classNames={{
          backdrop: 'bg-[#000]/0 backdrop-opacity-40',
          base: 'bg-[#b3b3b3] drop-shadow-2xl w-40 overflow-hidden',
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalBody>
                <fieldset className="border-1 border-black h-fit -ml-4 -mr-4 pt-2 pb-2 flex flex-col items-center justify-center gap-4">
                  {Runways.map((Runway) => (
                    <Button
                      key={Runway}
                      radius="none"
                      className="text-xl bg-[#ccc] text-blackdrop-shadow w-24 drop-shadow-md border-gray-500 border-1 border-opacity-25"
                    >
                      {Runway}
                    </Button>
                  ))}
                </fieldset>
                <div className="flex justify-center w-28">
                  <Button
                    radius="none"
                    size="lg"
                    className="text-xl bg-[#3F3F3F] text-white w-24"
                    onPress={onClose}
                  >
                    ESC
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
