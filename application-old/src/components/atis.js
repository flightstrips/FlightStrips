import React from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  useDisclosure,
} from "@nextui-org/react";

export const ATIS = (props) => {
  const { isOpen, onOpen, onOpenChange } = useDisclosure();
  return (
    <>
      <Button onPress={onOpen} radius="none" className="bg-[#646464] border-white border-2 w-fit h-12 pl-6 pr-6  ml-1 text-white text-3xl font-bold">ATIS</Button>
      <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="5xl" radius="none" classNames={{
        backdrop: "bg-[#000]/50 backdrop-opacity-40",
        base: "border-[#292f46] bg-[#e4e4e4] drop-shadow-2xl"
      }}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalBody>
                <div className="border-2 border-black h-72 mt-4 mb-4 flex items-center justify-center ">
                  <div className="flex flex-col justtify-center items-center">
                    <p className="p-2 text-lg">METAR</p>
                    <p className="flex  justify-center text-xl text-center bg-white pt-16 pb-16 pl-4 pr-4">
                      EKCH 041020Z AUTO 02020KT 9999 -SN BKN020/// BKN059/// M03/M07 Q1000 NOSIG
                    </p>
                  </div>
                </div>
              </ModalBody>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
};
