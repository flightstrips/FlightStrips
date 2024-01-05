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

export const RunwayConfig = (props) => {
  const { isOpen, onOpen, onOpenChange } = useDisclosure();

  function RwyBtn(props) {
    if (props.active) {
      return (
        <Button
          radius="none"
          size="lg"
          variant="solid"
          color="success"
          className="p-4 m-2 border-2 border-black w-full"
        >
          {props.runway}
        </Button>
      );
    } else {
      return (
        <Button
          radius="none"
          size="lg"
          variant="bordered"
          className="p-4 m-2 border-2 border-black w-full"
        >
          {props.runway}
        </Button>
      );
    }
  }
  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        className="bg-white w-fit h-12 ml-2 mr-2 pl-2 pr-2  flex items-center text-center text-3xl font-extrabold"
      >
        {props.runway}
      </Button>
      <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="xl">
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                EKCH - Runway Configuration
              </ModalHeader>
              <ModalBody>
                <div className="flex font-extrabold justify-evenly">

                  <div className="flex-col">
                    <p className="w-full text-center">DEP RWY</p>
                    <div className="w-full h-full border-2 border-black flex flex-col text-xl">
                      <div className="w-full justify-center items-center flex">
                        <RwyBtn runway="04L"/>
                        <RwyBtn runway="04R"/>
                      </div>
                      <div className="w-full justify-center items-center flex">
                        <RwyBtn runway="22L"/>
                        <RwyBtn runway="22R" active/>
                      </div>
                      <div className="w-full justify-center items-center flex">
                        <RwyBtn runway="12"/>
                        <RwyBtn runway="30"/>
                      </div>
                    </div>
                  </div>

                  <div className="flex-col">
                    <p className="w-full text-center">DEP RWY</p>
                    <div className="w-full h-full border-2 border-black flex flex-col text-xl">
                      <div className="w-full justify-center items-center flex">
                        <RwyBtn runway="04L"/>
                        <RwyBtn runway="04R"/>
                      </div>
                      <div className="w-full justify-center items-center flex">
                        <RwyBtn runway="22L" active/>
                        <RwyBtn runway="22R" />
                      </div>
                      <div className="w-full justify-center items-center flex">
                        <RwyBtn runway="12"/>
                        <RwyBtn runway="30"/>
                      </div>
                    </div>
                  </div>

                </div>
              </ModalBody>
              <ModalFooter>
                <Button
                  color="danger"
                  variant="light"
                  onPress={onClose}
                  className="mt-6"
                >
                  Close
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
};
