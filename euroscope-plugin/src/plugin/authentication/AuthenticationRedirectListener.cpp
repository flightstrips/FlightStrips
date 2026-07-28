#include "AuthenticationRedirectListener.h"

#include "ExceptionHandling.h"
#include "Logger.hpp"


namespace FlightStrips::authentication {
    namespace {
        constexpr char kPostLoginRedirectUrl[] = "https://flightstrips.dk/plugin-auth-complete";
    }

    AuthenticationRedirectListener::AuthenticationRedirectListener(
        std::promise<std::optional<std::string> > &prms, const int port) : port(port), resultPromise(std::move(prms)) {
    }

    bool AuthenticationRedirectListener::Start() {
        try {
            server.set_exception_handler([this](const httplib::Request&, httplib::Response &res, const std::exception_ptr ep) {
                const auto details = exceptions::GetExceptionDetails(ep);
                exceptions::LogException("AuthenticationRedirectListener::RequestHandler", details);
                res.status = 500;
                res.set_content("Authentication failed due to an internal error.", "text/plain; charset=utf-8");
                TrySetResult({}, "AuthenticationRedirectListener::RequestHandler");
            });

            server.Get("/callback-auth0", [this](const httplib::Request &request, httplib::Response &res) {
                if (!request.has_param("code")) {
                    Logger::Warning("Authentication callback did not contain code parameter");
                    res.status = 400;
                    TrySetResult({}, "AuthenticationRedirectListener::MissingCode");
                    return;
                }

                const auto code = request.get_param_value("code");
                res.set_redirect(kPostLoginRedirectUrl);
                TrySetResult(code, "AuthenticationRedirectListener::Callback");
            });

            auto startupFuture = startupPromise.get_future();
            this->backgroundThread = std::thread(&AuthenticationRedirectListener::BackgroundThread, this);
            if (!startupFuture.get()) {
                backgroundThread.join();
                return false;
            }

            server.wait_until_ready();
            if (server.is_running()) {
                return true;
            }

            if (backgroundThread.joinable()) {
                backgroundThread.join();
            }
            return false;
        } catch (...) {
            exceptions::LogCurrentException("AuthenticationRedirectListener::Start");
            Stop();
            TrySetResult({}, "AuthenticationRedirectListener::Start");
            return false;
        }
    }

    void AuthenticationRedirectListener::BackgroundThread() {
        try {
            if (!server.bind_to_port("127.0.0.1", port)) {
                Logger::Warning("Authentication redirect listener could not bind to port {}", port);
                TrySetStartupResult(false);
                return;
            }

            TrySetStartupResult(true);
            Logger::Debug(std::format("Starting HTTP server listing on http://127.0.0.1:{}", port));
            const auto listening = server.listen_after_bind();
            if (!listening && !resultSet.load()) {
                Logger::Error("Authentication redirect listener failed to start on port {}", port);
                TrySetResult({}, "AuthenticationRedirectListener::Listen");
            }
            Logger::Debug(std::format("Stopping HTTP server listing on http://127.0.0.1:{}", port));
        } catch (...) {
            TrySetStartupResult(false);
            exceptions::LogCurrentException("AuthenticationRedirectListener::BackgroundThread");
            TrySetResult({}, "AuthenticationRedirectListener::BackgroundThread");
        }
    }

    AuthenticationRedirectListener::~AuthenticationRedirectListener() {
        Stop();
    }

    void AuthenticationRedirectListener::Stop() {
        if (server.is_running()) {
            server.stop();
        }

        if (backgroundThread.joinable()) {
            backgroundThread.join();
        }
    }

    void AuthenticationRedirectListener::TrySetStartupResult(const bool value) {
        if (startupSet.exchange(true)) {
            return;
        }

        try {
            startupPromise.set_value(value);
        } catch (...) {
            exceptions::LogCurrentException("AuthenticationRedirectListener::TrySetStartupResult");
        }
    }

    void AuthenticationRedirectListener::TrySetResult(std::optional<std::string> value, const std::string& context) {
        if (resultSet.exchange(true)) {
            return;
        }

        try {
            resultPromise.set_value(std::move(value));
        } catch (...) {
            exceptions::LogCurrentException(context);
        }
    }
} // authentication
// FlightStrips
