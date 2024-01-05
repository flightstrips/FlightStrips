import React, { useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  ButtonGroup,
  useDisclosure,
  Input,
} from "@nextui-org/react";

export const NewVFRModal = (props) => {
  const { isOpen, onOpen, onOpenChange } = useDisclosure();
  const [cs, setCS] = useState("");
  const [actype, setActype] = useState("");

  return (
    <>
      <Button
        onPress={onOpen}
        radius="none"
        className="bg-[#646464] pl-4 pr-4 border-white border-2 mr-1 text-white text-xl"
      >
        NEW
      </Button>
      <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="lg">
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                NEW VFR FLIGHT
              </ModalHeader>
              <ModalBody>
                <div className="flex">
                  <div className="border-2 border-black w-full h-fit">
                    <div className="flex">
                      <Input
                        radius="none"
                        label="CALLSIGN/ REGISTRATION"
                        value={cs}
                        size="sm"
                        onChange={(event) =>
                          setCS(event.target.value.toUpperCase())
                        }
                      />
                      <Button radius="none" className="" size="lg">
                        ERASE
                      </Button>
                    </div>
                    <div className="flex">
                      <Input
                        label="AIRCRAFT TYPE"
                        radius="none"
                        size="sm"
                        value={actype}
                        onChange={(event) =>
                          setActype(event.target.value.toUpperCase())
                        }
                      />
                      <Button radius="none" className="" size="lg">
                        ERASE
                      </Button>
                    </div>
                    <div className="flex">
                      <Input
                        label="QNH"
                        radius="none"
                        size="sm"
                        value={1015}
                        readOnly
                      />
                      <Input
                        label="TRANSPONDER CODE"
                        radius="none"
                        size="sm"
                        defaultValue="7000"
                        maxLength={4}
                      />
                      <Button radius="none" className="" size="lg">
                        CYCLE
                      </Button>
                    </div>
                    <div className="flex">
                      <ButtonGroup radius="none" className="w-full" size="lg">
                        <Button className="w-full">
                          DK
                        </Button>
                        <Button className="w-full" >
                          EN
                        </Button>
                      </ButtonGroup>
                    </div>
                    <div className="flex flex-col">
                      <ButtonGroup radius="none" className="w-full" size="lg">
                        <Button className="w-full">CIRCUT</Button>
                        <Button className="w-full">RIGET</Button>
                        <Button className="w-full">SIGHT</Button>
                      </ButtonGroup>
                      <ButtonGroup radius="none" className="w-full" size="lg">
                        <Button className="w-full">VALLENSBÆK</Button>
                        <Button className="w-full">ELLERHAMMER</Button>
                        <Button className="w-full">TUBORG</Button>
                      </ButtonGroup>
                    </div>
                    <div className="flex">
                      <Input
                        label="REMARKS"
                        radius="none"
                        size="sm"
                      />
                      <Button radius="none" className="" size="lg">
                        ERASE
                      </Button>
                    </div>
                  </div>
                </div>
              </ModalBody>
              <ModalFooter className=" justify-center">
                <Button color="danger" variant="light" onPress={onClose}>
                  ESC
                </Button>
                <Button color="primary" onPress={onClose}>
                  OK
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
};
