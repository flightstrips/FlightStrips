//
// Created by fsr19 on 14/01/2025.
//

#pragma once

#define _WEBSOCKETPP_CPP11_RANDOM_DEVICE_
#define _WEBSOCKETPP_CPP11_TYPE_TRAITS_
#define ASIO_STANDALONE
#include <asio/asio.hpp>
#include <websocketpp/config/asio_client.hpp>
#include <websocketpp/client.hpp>


typedef std::function<void()> on_connected_callback;
typedef std::function<void(const std::string&)> message_callback;

typedef websocketpp::client<websocketpp::config::asio_client> client;

namespace FlightStrips::websocket {
    enum WebSocketStatus {
        WEBSOCKET_STATUS_DISCONNECTED,
        WEBSOCKET_STATUS_CONNECTING,
        WEBSOCKET_STATUS_CONNECTED,
        WEBSOCKET_STATUS_FAILED
    };

    class WebSocket {
    public:
        WebSocket(std::string  endpoint, message_callback  cb, on_connected_callback on_connected);
        ~WebSocket();
        void Connect();
        void Disconnect();
        void Send(const std::string& message) ;
        bool IsConnected() ;

    private:
        WebSocketStatus status_ = WEBSOCKET_STATUS_DISCONNECTED;
        client m_endpoint;
        websocketpp::lib::shared_ptr<websocketpp::lib::thread> m_thread;
        websocketpp::connection_hdl m_hdl;

        on_connected_callback on_connected_cb;
        message_callback message_cb;
        std::string endpoint;
        void OnMessage(const websocketpp::connection_hdl& hdl, const client::message_ptr &msg) const;
        void OnFailure(const websocketpp::connection_hdl &hdl) ;
        void OnOpen(const websocketpp::connection_hdl &hdl) ;

        void TryDisconnect();

        static void add_windows_root_certs(const std::shared_ptr<asio::ssl::context>& context);
    };

}
