#pragma once
#include <string>

namespace FlightStrips::util::http {

/// Pure-virtual interface for HTTP operations.
/// The production Http class implements this; tests use MockHttpClient.
class IHttpClient {
public:
    virtual ~IHttpClient() = default;

    virtual std::string Get(const std::string& url, const std::string& token = "") = 0;
    virtual std::string Post(const std::string& url, const std::string& body, const std::string& token = "") = 0;
};

} // namespace FlightStrips::util::http
