#pragma once

#include <flight_strips.grpc.pb.h>

#include <utility>
#include <grpcpp/grpcpp.h>


namespace FlightStrips::network {

    class Reader : public grpc::ClientBidiReactor<ClientStreamMessage, ServerStreamMessage> {
    public:
        Reader(FlightStripsService::Stub* stub, std::function<void(const ServerStreamMessage& message)> callBack) : callBack_(std::move(callBack)) {
            stub->async()->Start(&context_, this);
            StartRead(&response_);
            StartCall();
        }

        void OnWriteDone(bool ok) override {
            if (!ok) {
                // Error
                StartWritesDone();
                done = true;
                return;
            }

            NextWrite();
        }


        void OnReadDone(bool ok) override {
            if (!ok) {
                // Error
                return;
            }

            callBack_(response_);

            StartRead(&response_);
        }

        void AddMessage(const ClientStreamMessage& message) {
            if (done) return;
            //std::lock_guard<std::mutex> lock(mutex_);
            messages_.push(message);
            if (messages_.size() == 1) NextWrite();
        }

        void OnDone(const grpc::Status&) override {
            done = true;
            //delete this;
        }

        void TryCancel() {
            if (!done) {
                StartWritesDone();
                done = true;
            }
            context_.TryCancel();
        }

    private:
        bool done = false;
        grpc::ClientContext context_;
        ServerStreamMessage response_;
        std::mutex mutex_;
        std::queue<ClientStreamMessage> messages_;
        std::function<void(const ServerStreamMessage& message)> callBack_;

        void NextWrite() {
            std::lock_guard<std::mutex> lock(mutex_);
            if (messages_.empty()) return;

            auto msg = messages_.front();
            messages_.pop();
            StartWrite(&msg);
        }

    };

    class FlightStripsClient {
    public:
        explicit FlightStripsClient(const std::shared_ptr<grpc::Channel>& channel);
        ~FlightStripsClient();

        /*
         *
         */
        std::unique_ptr<Reader> StartConnection(const std::function<void(const ServerStreamMessage& message)>& callBack);
        void GetAllFlightStrips(Session s);

    private:
        std::unique_ptr<FlightStripsService::Stub> stub_;
    };


} // network
// FlightStrips
