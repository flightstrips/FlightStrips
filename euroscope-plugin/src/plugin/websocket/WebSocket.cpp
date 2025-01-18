//
// Created by fsr19 on 14/01/2025.
//

#include "WebSocket.h"

#include "Logger.h"
#include <asio/asio/ssl.hpp>

namespace FlightStrips::websocket {
    WebSocket::WebSocket(const std::string &endpoint, const callback& cb) : endpoint(endpoint), cb(cb) { }

    void WebSocket::Start() {
        if (connection || thread.joinable()) return;
        thread = std::thread(&WebSocket::Run, this);
    }

    void WebSocket::Stop() {
        if (IsConnected()) {
            websocketpp::lib::error_code ec;
            connection->close(websocketpp::close::status::going_away, "Shutting down", ec);
            if (ec) {
                Logger::Error(std::format("Error during close: {}", ec.message()));
            }
        }
        connection.reset();
        if (thread.joinable()) {
            thread.join();
        }
    }

    void WebSocket::Send(const std::string &message) const {
        if (!IsConnected()) {
            Logger::Debug("Trying to send message with websocket not connected");
            return;
        }

        connection->send(message, websocketpp::frame::opcode::text);
    }

    bool WebSocket::IsConnected() const {
        return connection && connection->get_state() == websocketpp::session::state::open;
    }

    void WebSocket::Run() {
        try {

            client c;

            c.init_asio();

            /*
            c.set_tls_init_handler([this](const websocketpp::connection_hdl&) {
                auto context = websocketpp::lib::make_shared<asio::ssl::context>(asio::ssl::context::tlsv12_client);
                context->set_options(asio::ssl::context::default_workarounds
                 | asio::ssl::context::no_sslv2
                 | asio::ssl::context::no_sslv3
                 | asio::ssl::context::no_tlsv1
                 | asio::ssl::context::no_tlsv1_1);
                add_windows_root_certs(context);
                context->set_verify_mode(asio::ssl::verify_peer | asio::ssl::verify_fail_if_no_peer_cert);
                return context;
            });
            */

            c.set_message_handler([this]<typename T0, typename T1>(T0 && hdl, T1 && msg) { OnMessage(std::forward<T0>(hdl), std::forward<T1>(msg)); });

            Logger::Debug("Get Connection...");

            websocketpp::lib::error_code ec;
            connection = c.get_connection(endpoint, ec);
            if (ec) {
                Logger::Error(std::format("Could not create connection: {}", ec.message()));
                return;
            }

            Logger::Debug("Connecting...");

            c.connect(connection);

            Logger::Debug("Run websocket...");

            c.run();

            Logger::Debug("Shutting down websocket...");
        } catch (const std::exception& e) {
            Logger::Error(e.what());
        }
    }

    void WebSocket::OnMessage(websocketpp::connection_hdl, const client::message_ptr &msg) const {
        auto payload = msg->get_payload();
        Logger::Debug("Got message from server: {}", payload);
        cb(payload);
    }

    void WebSocket::add_windows_root_certs(const std::shared_ptr<asio::ssl::context>& context) {
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
            if(x509 != nullptr) {
                X509_STORE_add_cert(store, x509);
                X509_free(x509);
            }
        }

        CertFreeCertificateContext(pContext);
        CertCloseStore(hStore, 0);

        SSL_CTX_set_cert_store(context->native_handle(), store);
    }
}
