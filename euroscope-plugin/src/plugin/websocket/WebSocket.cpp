#include "WebSocket.h"

#include "Logger.h"
#include <asio/asio/ssl.hpp>
#include <utility>

namespace FlightStrips::websocket {
    WebSocket::WebSocket(std::string endpoint, message_callback cb,
                         on_connected_callback on_connected) : on_connected_cb(std::move(on_connected)),
                                                               message_cb(std::move(cb)),
                                                               endpoint(std::move(endpoint)) {
        m_endpoint.clear_access_channels(websocketpp::log::alevel::all);
        m_endpoint.clear_error_channels(websocketpp::log::elevel::all);
        m_endpoint.init_asio();
        m_endpoint.set_close_handshake_timeout(1000);
        m_endpoint.set_open_handshake_timeout(1000);
        m_endpoint.start_perpetual();
        m_thread = websocketpp::lib::make_shared<websocketpp::lib::thread>(&client::run, &m_endpoint);
    }

    WebSocket::~WebSocket() {
        m_endpoint.stop_perpetual();
        Logger::Debug("WebSocket: Stopping");
        TryDisconnect();
        status_ = WEBSOCKET_STATUS_DISCONNECTED;
        Logger::Debug("WebSocket: Waiting for thread to join...");
        m_thread->join();
        Logger::Debug("WebSocket: Thread joined");
    }

    void WebSocket::TryDisconnect()
    {
        websocketpp::lib::error_code ec;
        const auto connection = m_endpoint.get_con_from_hdl(m_hdl, ec);
        if (!ec) {
            Logger::Debug("WebSocket: Connection open, closing");
            connection->close(websocketpp::close::status::going_away, "Stopping", ec);
            if (ec) {
                Logger::Info("WebSocket: Failed to stop websocket client: {}", ec.message());
            }
        } else {
            Logger::Debug("WebSocket: Failed to get connection: {}", ec.message());
        }
    }

    void WebSocket::Connect() {
        if (status_ != WEBSOCKET_STATUS_DISCONNECTED && status_ != WEBSOCKET_STATUS_FAILED) { return; }
        status_ = WEBSOCKET_STATUS_CONNECTING;
        websocketpp::lib::error_code ec;

        const client::connection_ptr con = m_endpoint.get_connection(endpoint, ec);
        if (ec) {
            Logger::Debug("Failed to connect: {}", ec.message());
            return;
        }

        m_hdl = con->get_handle();

        con->set_open_handler([this] (const websocketpp::connection_hdl &hdl) { OnOpen(hdl); });
        con->set_message_handler([this](const websocketpp::connection_hdl &hdl, const client::message_ptr &msg) { OnMessage(hdl, msg); });
        con->set_fail_handler([this](const websocketpp::connection_hdl &hdl) { OnFailure(hdl); });
        con->set_close_handler([this](const websocketpp::connection_hdl &hdl) { OnClose(hdl); });

        m_endpoint.connect(con);
    }

    void WebSocket::Disconnect() {
        TryDisconnect();
        status_ = WEBSOCKET_STATUS_DISCONNECTED;
        m_hdl.reset();
    }

    void WebSocket::Send(const std::string &message) {
        websocketpp::lib::error_code ec;
        m_endpoint.send(m_hdl, message, websocketpp::frame::opcode::text, ec);
        if (ec) {
            Logger::Warning("Failed to send message '{}' with error code {}: {}.", message, ec.value(), ec.message());
        }
    }

    WebSocketStatus WebSocket::GetStatus() const {
        return status_;
    }

    void WebSocket::OnMessage(const websocketpp::connection_hdl &, const client::message_ptr &msg) const {
        auto payload = msg->get_payload();
        Logger::Debug("Got message from server: {}", payload);
        message_cb(payload);
    }

    void WebSocket::OnFailure(const websocketpp::connection_hdl &hdl) {
        status_ = WEBSOCKET_STATUS_FAILED;
        const client::connection_ptr con = m_endpoint.get_con_from_hdl(hdl);
        const auto m_error_reason = con->get_ec().message();
        Logger::Warning("Failed to connect to server: {}",  m_error_reason);
    }

    void WebSocket::OnOpen(const websocketpp::connection_hdl &hdl) {
        status_ = WEBSOCKET_STATUS_CONNECTED;
        const client::connection_ptr con = m_endpoint.get_con_from_hdl(hdl);
        auto server = con->get_response_header("Server");
        Logger::Info("Connected to server: {}", server);
        on_connected_cb();
    }

    void WebSocket::OnClose(const websocketpp::connection_hdl &hdl) {
        status_ = WEBSOCKET_STATUS_DISCONNECTED;
        const client::connection_ptr con = m_endpoint.get_con_from_hdl(hdl);
        const auto m_error_reason = con->get_ec().message();
        Logger::Debug("Connection to server closed. Reason: {}", m_error_reason);
    }

    void WebSocket::add_windows_root_certs(const std::shared_ptr<asio::ssl::context> &context) {
        HCERTSTORE hStore = CertOpenSystemStore(0, L"ROOT");
        if (hStore == nullptr) {
            return;
        }

        X509_STORE *store = X509_STORE_new();
        PCCERT_CONTEXT pContext = nullptr;
        while ((pContext = CertEnumCertificatesInStore(hStore, pContext)) != nullptr) {
            X509 *x509 = d2i_X509(nullptr,
                                  const_cast<const unsigned char **>(&pContext->pbCertEncoded),
                                  pContext->cbCertEncoded);
            if (x509 != nullptr) {
                X509_STORE_add_cert(store, x509);
                X509_free(x509);
            }
        }

        CertFreeCertificateContext(pContext);
        CertCloseStore(hStore, 0);

        SSL_CTX_set_cert_store(context->native_handle(), store);
    }
}
