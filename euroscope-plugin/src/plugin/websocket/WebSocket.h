//
// Created by fsr19 on 14/01/2025.
//

#pragma once

#define _WEBSOCKETPP_CPP11_RANDOM_DEVICE_
#define _WEBSOCKETPP_CPP11_TYPE_TRAITS_
#include <asio/asio.hpp>
#include <websocketpp/config/asio_client.hpp>
#include <websocketpp/client.hpp>


typedef std::function<void(const std::string&)> callback;

typedef websocketpp::client<websocketpp::config::asio_client> client;

namespace FlightStrips::websocket {
    class WebSocket {
    public:
        WebSocket(const std::string& endpoint, const callback& cb);
        void Start();
        void Stop();
        void Send(const std::string& message) const;
        bool IsConnected() const;

    private:
        callback cb;
        std::string endpoint;
        std::thread thread;
        client::connection_ptr connection;
        void Run();

        void OnMessage(websocketpp::connection_hdl hdl, const client::message_ptr &msg) const;

        static void add_windows_root_certs(const std::shared_ptr<asio::ssl::context>& context);
    };

}
