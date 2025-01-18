#pragma once

#include <httplib/httplib.h>
#include <thread>
#include <future>
#include <optional>

namespace FlightStrips::authentication {

class AuthenticationRedirectListener {
public:
    AuthenticationRedirectListener(std::promise<std::optional<std::string>>& prms, int port);
    ~AuthenticationRedirectListener();
    void Stop();
    void Start();

private:
    int port;
    httplib::Server server;
    std::thread backgroundThread;
    std::promise<std::optional<std::string>> resultPromise;

    void BackgroundThread();
};

} // authentication
// FlightStrips
