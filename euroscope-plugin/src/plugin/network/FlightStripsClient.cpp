#include "FlightStripsClient.h"

namespace FlightStrips {
    namespace network {

        FlightStripsClient::FlightStripsClient(const std::shared_ptr<grpc::Channel> &channel) : stub_(FlightStripsService::NewStub(channel)) {
        }

        std::unique_ptr<Reader> FlightStripsClient::StartConnection(const std::function<void(const ServerStreamMessage& message)>& callBack) {
            return std::make_unique<Reader>(stub_.get(), callBack);
        }

        void FlightStripsClient::GetAllFlightStrips(Session) {

        }

        FlightStripsClient::~FlightStripsClient() {
            stub_.reset();
        }
    } // network
} // FlightStrips