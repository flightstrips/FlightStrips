import React from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  useDisclosure,
  Textarea,
  Input,
} from "@nextui-org/react";

export const MSGModal = (props) => {
  const { isOpen, onOpen, onOpenChange } = useDisclosure();

  function BroadcastToStation(props) {
    return (
      <Button radius="none" className="w-24 m-2 font-semibold">{props.text}</Button>
    )
  }

  function PreDefMSG(props) {
    return (
      <Button radius="none" className="w-full mt-1 mb-1 font-semibold drop-shadow-md text-lg justify-items-start text-left">{props.text}</Button>
    )
  }

  return (
    <>
      <Button onPress={onOpen} radius="none" className="bg-[#646464] pl-4 pr-4 border-white border-2 mr-1 text-white text-xl">NEW</Button>
      <Modal isOpen={isOpen} onOpenChange={onOpenChange} size="5xl" radius="none" classNames={{
        backdrop: "bg-[#000]/75 backdrop-opacity-40",
        base: "border-[#292f46] bg-[#e4e4e4] text-[#a8b0d3] drop-shadow-2xl"
      }}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalBody className="border-1 border-black m-4">
                <div className="flex justify-center items-center flex-col">

                  <div className="w-5/6 mb-8">
                    <p className="text-center text-black pl-4 pt-4 pr-4">FREE TEXT</p>
                    <Input placeholder="Text can be written down here" radius="none" size="lg" classNames={{ input: "text-2xl text-center" }} />
                  </div>


                  <div className="flex justify-evenly">

                    <div className="flex w-1/3 flex-col justify-center items-center">
                      <div className="bg-gray-500 p-4">
                        <Button radius="none" color="success" className="w-52 ml-4 mr-4 mb-2 mt-2 font-semibold">BROADCAST</Button>
                        <div className="flex items-start">
                          <BroadcastToStation text="CLR DEL" />
                          <BroadcastToStation text="CLR DEL" />
                        </div>
                        <div className="flex">
                          <BroadcastToStation text="APRON ARR" />
                          <BroadcastToStation text="APRON DEP" />
                        </div>
                        <div className="flex">
                          <BroadcastToStation text="GND WEST" />
                          <BroadcastToStation text="GND EAST" />
                        </div>
                        <div className="flex">
                          <BroadcastToStation text="TWR ARR" />
                          <BroadcastToStation text="TWR DEP" />
                        </div>
                      </div>
                    </div>

                    <div className="w-2/3">
                      <PreDefMSG text="RUNWAY CHANGE TO 04R/04L" />
                      <PreDefMSG text="RUNWAY CHANGE TO 22R/22L" />
                      <PreDefMSG text="RUNWAY CHANGE TO 12/30" />
                      <PreDefMSG text="CLOSING POSITION SOON" />
                      <PreDefMSG text="ENFORCE A-CDM. TRAFFIC LOAD TOO HIGH" />
                      <PreDefMSG text="ALL DEPARTURES MUST BE CLRD RWY-HDG 4000' UFN" />
                      <PreDefMSG text="ATIS REPORTED DOWN. PLEASE RESOLVE" />
                      <PreDefMSG text="ATIS REPORT WRONG RWY CONFIG" />
                    </div>

                  </div>
                </div>
                <div className="w-full flex justify-center">
                  <Button radius="none" className="bg-[#3f3f3f] text-white text-2xl p-2 ml-4 mr-4" onPress={onClose}>
                    ERASE
                  </Button>
                  <Button radius="none" className="bg-[#3f3f3f] text-white text-2xl p-2 ml-4 mr-4" onPress={onClose}>
                    OK
                  </Button>
                </div>
              </ModalBody>
              <ModalFooter className="justify-evenly">
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
};
