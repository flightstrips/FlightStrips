#include "WebSocket.h"

#include "Logger.h"

#include <asio/asio/ssl.hpp>
#include <utility>

namespace FlightStrips::websocket {

    namespace {
        using plain_client = websocketpp::client<websocketpp::config::asio_client>;
        using tls_client   = websocketpp::client<websocketpp::config::asio_tls_client>;

        template <typename ClientT, bool Tls>
        class WebSocketImpl final : public WebSocket::ImplBase {
        public:
            WebSocketImpl(std::string endpoint,
                          message_callback cb,
                          on_connected_callback on_connected)
                : on_connected_cb(std::move(on_connected)),
                  message_cb(std::move(cb)),
                  endpoint_(std::move(endpoint)) {

                m_endpoint.clear_access_channels(websocketpp::log::alevel::all);
                m_endpoint.clear_error_channels(websocketpp::log::elevel::all);
                m_endpoint.init_asio();
                m_endpoint.set_close_handshake_timeout(1000);
                m_endpoint.set_open_handshake_timeout(1000);

                if constexpr (Tls) {
                    m_endpoint.set_tls_init_handler([this](websocketpp::connection_hdl) {
                        auto ctx = std::make_shared<asio::ssl::context>(asio::ssl::context::tls_client);

                        ctx->set_options(
                            asio::ssl::context::default_workarounds |
                            asio::ssl::context::no_sslv2 |
                            asio::ssl::context::no_sslv3 |
                            asio::ssl::context::single_dh_use
                        );

                        // Trust Windows root store (your existing helper)
                        WebSocket::add_windows_root_certs(ctx);

                        // Enable certificate verification
                        ctx->set_verify_mode(asio::ssl::verify_peer);

                        return ctx;
                    });
                }

                m_endpoint.start_perpetual();
                m_thread = websocketpp::lib::make_shared<websocketpp::lib::thread>(&ClientT::run, &m_endpoint);
            }

            ~WebSocketImpl() override {
                m_endpoint.stop_perpetual();
                Logger::Debug("WebSocket: Stopping");
                TryDisconnect();
                status_ = WEBSOCKET_STATUS_DISCONNECTED;
                Logger::Debug("WebSocket: Waiting for thread to join...");
                m_thread->join();
                Logger::Debug("WebSocket: Thread joined");
            }

            void Connect() override {
                if (status_ != WEBSOCKET_STATUS_DISCONNECTED && status_ != WEBSOCKET_STATUS_FAILED) { return; }
                status_ = WEBSOCKET_STATUS_CONNECTING;

                websocketpp::lib::error_code ec;
                const auto con = m_endpoint.get_connection(endpoint_, ec);
                if (ec) {
                    Logger::Debug("Failed to connect: {}", ec.message());
                    status_ = WEBSOCKET_STATUS_FAILED;
                    return;
                }

                m_hdl = con->get_handle();

                con->set_open_handler([this](const websocketpp::connection_hdl& hdl) { OnOpen(hdl); });
                con->set_message_handler([this](const websocketpp::connection_hdl& hdl, const typename ClientT::message_ptr& msg) { OnMessage(hdl, msg); });
                con->set_fail_handler([this](const websocketpp::connection_hdl& hdl) { OnFailure(hdl); });
                con->set_close_handler([this](const websocketpp::connection_hdl& hdl) { OnClose(hdl); });

                m_endpoint.connect(con);
            }

            void Disconnect() override {
                TryDisconnect();
                status_ = WEBSOCKET_STATUS_DISCONNECTED;
                m_hdl.reset();
            }

            void Send(const std::string& message) override {
                websocketpp::lib::error_code ec;
                m_endpoint.send(m_hdl, message, websocketpp::frame::opcode::text, ec);
                if (ec) {
                    Logger::Warning("Failed to send message '{}' with error code {}: {}.", message, ec.value(), ec.message());
                }
            }

            [[nodiscard]] WebSocketStatus GetStatus() const override { return status_; }

        private:
            void TryDisconnect() {
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

            void OnMessage(const websocketpp::connection_hdl&, const typename ClientT::message_ptr& msg) const {
                auto payload = msg->get_payload();
                Logger::Debug("Got message from server: {}", payload);
                message_cb(payload);
            }

            void OnFailure(const websocketpp::connection_hdl& hdl) {
                status_ = WEBSOCKET_STATUS_FAILED;
                const auto con = m_endpoint.get_con_from_hdl(hdl);
                const auto reason = con->get_ec().message();
                Logger::Warning("Failed to connect to server: {}", reason);
            }

            void OnOpen(const websocketpp::connection_hdl& hdl) {
                status_ = WEBSOCKET_STATUS_CONNECTED;
                const auto con = m_endpoint.get_con_from_hdl(hdl);
                auto server = con->get_response_header("Server");
                Logger::Info("Connected to server: {}", server);
                on_connected_cb();
            }

            void OnClose(const websocketpp::connection_hdl& hdl) {
                status_ = WEBSOCKET_STATUS_DISCONNECTED;
                const auto con = m_endpoint.get_con_from_hdl(hdl);
                const auto reason = con->get_ec().message();
                Logger::Debug("Connection to server closed. Reason: {}", reason);
            }

            WebSocketStatus status_ = WEBSOCKET_STATUS_DISCONNECTED;

            ClientT m_endpoint;
            websocketpp::lib::shared_ptr<websocketpp::lib::thread> m_thread;
            websocketpp::connection_hdl m_hdl;

            on_connected_callback on_connected_cb;
            message_callback message_cb;
            std::string endpoint_;
        };
    }

    bool WebSocket::is_tls_endpoint(const std::string& endpoint) {
        return endpoint.rfind("wss://", 0) == 0;
    }

    WebSocket::WebSocket(std::string endpoint,
                         message_callback cb,
                         on_connected_callback on_connected) {
        if (is_tls_endpoint(endpoint)) {
            impl_ = std::make_unique<WebSocketImpl<tls_client, true>>(std::move(endpoint), std::move(cb), std::move(on_connected));
        } else {
            impl_ = std::make_unique<WebSocketImpl<plain_client, false>>(std::move(endpoint), std::move(cb), std::move(on_connected));
        }
    }

    WebSocket::~WebSocket() = default;

    void WebSocket::Connect() const { impl_->Connect(); }
    void WebSocket::Disconnect() const { impl_->Disconnect(); }
    void WebSocket::Send(const std::string& message) const { impl_->Send(message); }
    WebSocketStatus WebSocket::GetStatus() const { return impl_->GetStatus(); }

    void WebSocket::add_windows_root_certs(const std::shared_ptr<asio::ssl::context>& context) {
        HCERTSTORE hStore = CertOpenSystemStore(0, L"ROOT");
        if (hStore == nullptr) {
            return;
        }

        X509_STORE* store = X509_STORE_new();
        PCCERT_CONTEXT pContext = nullptr;
        while ((pContext = CertEnumCertificatesInStore(hStore, pContext)) != nullptr) {
            X509* x509 = d2i_X509(nullptr,
                                  const_cast<const unsigned char**>(&pContext->pbCertEncoded),
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
