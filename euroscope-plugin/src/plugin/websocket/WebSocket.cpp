//
// Created by fsr19 on 14/01/2025.
//

#include "WebSocket.h"

#include "Logger.h"
#include <asio/asio/ssl.hpp>

namespace FlightStrips::websocket {
    void WebSocket::Start() {
        thread = std::thread(&WebSocket::Run, this);
    }

    void WebSocket::Stop() {
        if (connection) {
            connection->close(websocketpp::close::status::going_away, "Shutting down");
            connection.reset();
        }
        if (thread.joinable()) {
            thread.join();
        }
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

            Logger::Info("Get Connection...");

            websocketpp::lib::error_code ec;
            connection = c.get_connection("ws://localhost:2994/euroscopeEvents", ec);
            if (ec) {
                Logger::Error(std::format("Could not create connection: {}", ec.message()));
                return;
            }

            Logger::Info("Connecting...");

            c.connect(connection);

            Logger::Info("Run...");

            c.run();

            Logger::Info("Shutting down...");
        } catch (const std::exception& e) {
            Logger::Error(e.what());
        }
    }

    void WebSocket::OnMessage(websocketpp::connection_hdl hdl, client::message_ptr msg) {
            std::stringstream s;
            s << "Got message: " << msg->get_payload();
            Logger::Info(s.str());
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
