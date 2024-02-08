import {
  Modal,
  ModalContent,
  ModalBody,
  Button,
  useDisclosure,
} from '@nextui-org/react'

export function CFLSelector(props: { cfl: string }) {
  const { isOpen, onOpen, onOpenChange } = useDisclosure()
  const CFLs = ['1500', '2500', '3000', '4000', '5000', 'FL070']
  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        className="border-1 border-black w-full h-full bg-default-100"
      >
        {props.cfl}
      </Button>
      <Modal
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        size="xs"
        classNames={{
          backdrop: 'bg-[#000]/0 backdrop-opacity-40 w-screen h-screen z-10',
          base: 'bg-[#D6D6D6] drop-shadow-2xl',
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalBody>
                <fieldset className="border-2 border-black h-fit mt-4 mb-4 flex flex-col items-center justify-center gap-4 pt-4 pb-4">
                  {CFLs.map((CFL) => (
                    <Button
                      key={CFL}
                      radius="none"
                      className="text-xl bg-[#d6d6d6] text-blackdrop-shadow w-32 drop-shadow-md border-gray-500 border-1 border-opacity-25"
                    >
                      {CFL}
                    </Button>
                  ))}
                </fieldset>
                <div className="flex justify-between w-64">
                  <Button
                    radius="none"
                    size="lg"
                    className="text-xl bg-[#3F3F3F] text-white m-4 w-full"
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
                    ERASE
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
