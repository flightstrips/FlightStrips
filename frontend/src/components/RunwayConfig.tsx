import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  useDisclosure,
} from '@nextui-org/react'

export const RunwayConfig = (props: { ActiveRunway: string }) => {
  const { isOpen, onOpen, onOpenChange } = useDisclosure()

  function RwyBtn(props: { runway: string; Active?: boolean }) {
    if (props.Active) {
      return (
        <Button
          radius="none"
          size="lg"
          variant="solid"
          color="success"
          className="border-2 border-black w-full bg-[#70ed45]"
        >
          {props.runway}
        </Button>
      )
    } else {
      return (
        <Button
          radius="none"
          size="lg"
          variant="bordered"
          className="border-2 border-black w-full bg-[#d6d6d6]"
        >
          {props.runway}
        </Button>
      )
    }
  }
  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        className="bg-white w-fit h-12 ml-2 mr-2 pl-2 pr-2  flex items-center text-center text-3xl font-extrabold"
      >
        {props.ActiveRunway}
      </Button>
      <Modal
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        radius="none"
        size="xl"
        classNames={{
          backdrop: 'bg-[#000]/50 backdrop-opacity-40 w-screen h-screen z-10',
          base: 'bg-[#b3b3b3] drop-shadow-2xl',
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalBody>
                <div className="flex font-semibold justify-evenly">
                  <fieldset className="flex-col">
                    <legend className="w-full text-center">DEP RWY</legend>
                    <div className="w-full h-full border-2 border-black flex flex-col text-xl">
                      <div className="w-full justify-center items-center flex gap-4 p-4">
                        <RwyBtn runway="04L" />
                        <RwyBtn runway="04R" />
                      </div>
                      <div className="w-full justify-center items-center flex gap-4 p-4">
                        <RwyBtn runway="22L" />
                        <RwyBtn runway="22R" Active />
                      </div>
                      <div className="w-full justify-center items-center flex gap-4 p-4">
                        <RwyBtn runway="12" />
                        <RwyBtn runway="30" />
                      </div>
                    </div>
                  </fieldset>

                  <div className="flex-col">
                    <p className="w-full text-center">DEP RWY</p>
                    <div className="w-full h-full border-2 border-black flex flex-col text-xl">
                      <div className="w-full justify-center items-center flex gap-4 p-4">
                        <RwyBtn runway="04L" />
                        <RwyBtn runway="04R" />
                      </div>
                      <div className="w-full justify-center items-center flex gap-4 p-4">
                        <RwyBtn runway="22L" Active />
                        <RwyBtn runway="22R" />
                      </div>
                      <div className="w-full justify-center items-center flex gap-4 p-4">
                        <RwyBtn runway="12" />
                        <RwyBtn runway="30" />
                      </div>
                    </div>
                  </div>
                </div>
              </ModalBody>
              <ModalFooter className="flex justify-center">
                <Button
                  radius="none"
                  size="lg"
                  className="text-xl bg-[#3F3F3F] text-white m-4"
                  onPress={onClose}
                >
                  OK
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  )
}
