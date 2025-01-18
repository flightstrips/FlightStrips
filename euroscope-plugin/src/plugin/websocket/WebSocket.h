//
// Created by fsr19 on 14/01/2025.
//

#pragma once

#define _WEBSOCKETPP_CPP11_RANDOM_DEVICE_
#define _WEBSOCKETPP_CPP11_TYPE_TRAITS_
#include <asio/asio.hpp>
#include "websocketpp/config/asio_client.hpp"
#include "websocketpp/client.hpp"

typedef websocketpp::client<websocketpp::config::asio_client> client;

namespace FlightStrips::websocket {
    class WebSocket {
    public:
        void Start();
        void Stop();

    private:
        std::thread thread;
        client::connection_ptr connection;
        void Run();

        void OnMessage(websocketpp::connection_hdl hdl, client::message_ptr msg);

        static void add_windows_root_certs(const std::shared_ptr<asio::ssl::context>& context);
    };

}
