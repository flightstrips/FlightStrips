#pragma once

#include <httplib/httplib.h>
#include <thread>
#include <future>
#include <optional>
#include <atomic>

namespace FlightStrips::authentication {

class AuthenticationRedirectListener {
public:
    AuthenticationRedirectListener(std::promise<std::optional<std::string>>& prms, int port);
    ~AuthenticationRedirectListener();
    void Stop();
    [[nodiscard]] bool Start();

private:
    int port;
    httplib::Server server;
    std::thread backgroundThread;
    std::promise<std::optional<std::string>> resultPromise;
    std::promise<bool> startupPromise;
    std::atomic_bool resultSet = false;
    std::atomic_bool startupSet = false;

    void BackgroundThread();
    void TrySetStartupResult(bool value);
    void TrySetResult(std::optional<std::string> value, const std::string& context);
};

} // authentication
// FlightStrips
