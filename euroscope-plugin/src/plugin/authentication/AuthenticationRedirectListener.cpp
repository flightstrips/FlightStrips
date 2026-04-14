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

    void AuthenticationRedirectListener::Start() {
        try {
            this->backgroundThread = std::thread(&AuthenticationRedirectListener::BackgroundThread, this);
        } catch (...) {
            exceptions::LogCurrentException("AuthenticationRedirectListener::Start");
            TrySetResult({}, "AuthenticationRedirectListener::Start");
        }
    }

    void AuthenticationRedirectListener::BackgroundThread() {
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

            Logger::Debug(std::format("Starting HTTP server listing on http://127.0.0.1:{}", port));
            const auto listening = server.listen("127.0.0.1", port);
            if (!listening && !resultSet.load()) {
                Logger::Error("Authentication redirect listener failed to start on port {}", port);
                TrySetResult({}, "AuthenticationRedirectListener::Listen");
            }
            Logger::Debug(std::format("Stopping HTTP server listing on http://127.0.0.1:{}", port));
        } catch (...) {
            exceptions::LogCurrentException("AuthenticationRedirectListener::BackgroundThread");
            TrySetResult({}, "AuthenticationRedirectListener::BackgroundThread");
        }
    }

    AuthenticationRedirectListener::~AuthenticationRedirectListener() = default;

    void AuthenticationRedirectListener::Stop() {
        if (server.is_running()) {
            server.stop();
        }

        if (backgroundThread.joinable()) {
            backgroundThread.join();
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
