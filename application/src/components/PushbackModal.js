import React from "react";
import {Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, Button, useDisclosure} from "@nextui-org/react";
import { PushBtn } from "./PushBtn";


export default function PushbackModal() {
    const {isOpen, onOpen, onOpenChange} = useDisclosure();
  
    return (
      <>
        <Button onPress={onOpen}>Open Modal</Button>
        <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="full" radius="none" classNames={{
          body: "w-3/3 bg-transparent",
        }}>
          <ModalContent className="bg-transparent">
            {(onClose) => (
              <>
                <ModalBody className="bg-transparent p-0 m-0">
                    <PushBtn className="absolute top-[10%] left-[20%] z-500 p-0 m-0 w-4!important" radius="none">J4</PushBtn>
                    <img src="/img/ekch/pushback.jpg" className=""/>
                </ModalBody>
              </>
            )}
          </ModalContent>
        </Modal>
      </>
    );
  }
  