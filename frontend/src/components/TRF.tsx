import {
  Modal,
  ModalContent,
  ModalBody,
  Button,
  useDisclosure,
} from '@nextui-org/react'

// eslint-disable-next-line react-refresh/only-export-components
export function TRF() {
  const { isOpen, onOpen, onOpenChange } = useDisclosure()

  const freq = [
    { position: 'EKCH_A_GND', freq: '121.630' },
    { position: 'EKCH_D_GND', freq: '121.730' },
    { position: 'EKCH_C_TWR', freq: '118.580' },
    { position: 'EKCH_GE_TWR', freq: '121.730' },
    { position: 'EKCH_A_TWR', freq: '118.105' },
    { position: 'EKCH_D_TWR', freq: '119.355' },
    { position: 'EKCH_R_DEP', freq: '120.255' },
    { position: 'EKCH_K_DEP', freq: '124.980' },
    { position: 'EKCH_W_APP', freq: '119.805' },
    { position: 'EKCH_O_APP', freq: '118.455' },
    { position: 'EKDK_CTR', freq: '136.485' },
  ]
  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        className="bg-[#646464] border-white border-2 w-fit h-12 pl-2 pr-2 ml-1 text-white text-3xl font-bold"
      >
        RRF
      </Button>
      <Modal
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        size="sm"
        radius="none"
        classNames={{
          backdrop: 'bg-[#000]/50 backdrop-opacity-40 w-screen h-screen',
          base: 'border-[#292f46] bg-[#e4e4e4] drop-shadow-2xl',
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalBody>
                <div className="border-2 border-black h-auto mt-4 mb-4 flex items-center justify-center ">
                  <div className="flex justify-center items-center flex-wrap">
                    {freq.map((item) => (
                      <Button
                        radius="none"
                        size="md"
                        className="w-32 m-2"
                        key={item.position}
                      >
                        {item.position}
                        <br />
                        {item.freq}
                      </Button>
                    ))}
                  </div>
                </div>
                <Button
                  radius="none"
                  size="lg"
                  className="text-xl bg-[#3F3F3F] text-white m-4"
                  onPress={onClose}
                >
                  ESC
                </Button>
              </ModalBody>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  )
}
