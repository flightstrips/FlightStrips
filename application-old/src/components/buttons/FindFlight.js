import React from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  useDisclosure,
  Input,
} from "@nextui-org/react";

export const FindFlight = (props) => {
  const { isOpen, onOpen, onOpenChange } = useDisclosure();

  return (
    <>
      <Button onPress={onOpen} radius="none" className="bg-[#646464] pl-4 pr-4 border-white border-2 mr-1 text-white text-xl">FIND</Button>
      <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="xs" radius="none" classNames={{
        body: "py-6",
        backdrop: "bg-[#000]/50 backdrop-opacity-40",
        base: "border-[#292f46] bg-[#e4e4e4] text-[#a8b0d3] drop-shadow-2xl"
      }}>
        <ModalContent radius="none">
          {(onClose) => (
            <>
              <ModalBody>
                <h1 className="text-2xl w-full text-center font-bold text-[#3F3F3F]">C/S</h1>
                <Input className="drop-shadow" size="lg" radius="none" classNames="text-lg">

                </Input>
              </ModalBody>
              <ModalFooter className=" justify-center">
                <Button radius="none" size="lg" className="text-xl bg-[#3F3F3F] text-white m-4" onPress={onClose}>
                  ESC
                </Button>
                <Button radius="none" size="lg" className="text-xl bg-[#3F3F3F] text-white m-4" onPress={onClose}>
                  SEARCH
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
};
