#pragma once

#define _WEBSOCKETPP_CPP11_RANDOM_DEVICE_
#define _WEBSOCKETPP_CPP11_TYPE_TRAITS_
#define ASIO_STANDALONE
#include <asio/asio.hpp>
#include <websocketpp/client.hpp>
#include <websocketpp/config/asio_client.hpp>

#include <functional>
#include <memory>
#include <string>

typedef std::function<void()> on_connected_callback;
typedef std::function<void(const std::string&)> message_callback;

namespace FlightStrips::websocket {
    enum WebSocketStatus {
        WEBSOCKET_STATUS_DISCONNECTED,
        WEBSOCKET_STATUS_CONNECTING,
        WEBSOCKET_STATUS_CONNECTED,
        WEBSOCKET_STATUS_FAILED
    };

    class WebSocket {
    public:
        WebSocket(std::string endpoint, message_callback cb, on_connected_callback on_connected);
        ~WebSocket();

        void Connect() const;
        void Disconnect() const;
        void Send(const std::string& message) const;
        [[nodiscard]] WebSocketStatus GetStatus() const;

        struct ImplBase {
            virtual ~ImplBase() = default;
            virtual void Connect() = 0;
            virtual void Disconnect() = 0;
            virtual void Send(const std::string& message) = 0;
            [[nodiscard]] virtual WebSocketStatus GetStatus() const = 0;
        };

        static void add_windows_root_certs(const std::shared_ptr<asio::ssl::context>& context);

    private:

        std::unique_ptr<ImplBase> impl_;

        static bool is_tls_endpoint(const std::string& endpoint);
    };
}
