import {
  Modal,
  ModalContent,
  ModalBody,
  Button,
  useDisclosure,
} from '@nextui-org/react'

export function SIDButton(props: { SID: string }) {
  const { isOpen, onOpen, onOpenChange } = useDisclosure()
  const SIDs = [
    'LANGO2C',
    'NEXEN2C',
    'KOPEX2C',
    'ODN2C',
    'GOLGA2C',
    'VEDAR2C',
    'KEMAX2C',
    'SIMEG8C',
    'SALLO1C',
    'BETUD2C',
  ]
  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        className="border-1 border-black w-28 h-full"
      >
        {props.SID}
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
                  {SIDs.map((SIDName) => (
                    <Button
                      key={SIDName}
                      radius="none"
                      className="text-xl bg-[#d6d6d6] text-blackdrop-shadow w-32 drop-shadow-md border-gray-500 border-1 border-opacity-25"
                    >
                      {SIDName}
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
