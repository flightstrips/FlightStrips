#include "AuthenticationRedirectListener.h"

#include "Logger.h"


namespace FlightStrips::authentication {
    AuthenticationRedirectListener::AuthenticationRedirectListener(
        std::promise<std::optional<std::string> > &prms, const int port) : port(port), resultPromise(std::move(prms)) {
    }

    void AuthenticationRedirectListener::Start() {
        this->backgroundThread = std::thread(&AuthenticationRedirectListener::BackgroundThread, this);
    }

    void AuthenticationRedirectListener::BackgroundThread() {
        server.Get("/callback-auth0", [this](const httplib::Request &request, httplib::Response &res) {
            if (!request.has_param("code")) {
                Logger::Warning("Authentication callback did not contain code parameter");
                res.status = 400;
                resultPromise.set_value({});
            } else {
                const auto code = request.get_param_value("code");
                res.set_content("<html><body>Authentication successful, you may now close this window. Window will close after 5 seconds automatically.<script>setTimeout(function(){window.close()},5000);</script></body></html>", "text/html; charset=utf-8");
                resultPromise.set_value(code);
            }
        });

        Logger::Debug(std::format("Starting HTTP server listing on http://127.0.0.1:{}", port));
        server.listen("127.0.0.1", port);
        Logger::Debug(std::format("Stopping HTTP server listing on http://127.0.0.1:{}", port));
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
} // authentication
// FlightStrips
