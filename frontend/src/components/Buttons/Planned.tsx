import {
  Modal,
  ModalContent,
  ModalBody,
  ModalFooter,
  Button,
  useDisclosure,
  Input,
} from '@nextui-org/react'

export function Planned() {
  const { isOpen, onOpen, onOpenChange } = useDisclosure()

  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        size="sm"
        className="bg-[#646464] border-white border-2 mr-1 text-white text-md"
      >
        Planned
      </Button>
      <Modal
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        size="xs"
        radius="none"
        classNames={{
          body: 'py-6',
          backdrop: 'bg-[#000]/50 backdrop-opacity-40',
          base: 'border-[#292f46] bg-[#e4e4e4] text-[#a8b0d3] drop-shadow-2xl',
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalBody>
                <h1 className="text-2xl w-full text-center font-bold text-[#3F3F3F]">
                  C/S
                </h1>
                <Input
                  classNames={{
                    input: ['text-xl text-center'],
                    inputWrapper: ['drop-shadow'],
                  }}
                  size="lg"
                  radius="none"
                ></Input>
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
}
