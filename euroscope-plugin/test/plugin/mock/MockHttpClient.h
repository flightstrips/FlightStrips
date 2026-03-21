#pragma once
#include <gmock/gmock.h>
#include "util/http/IHttpClient.h"

class MockHttpClient : public FlightStrips::util::http::IHttpClient {
public:
    MOCK_METHOD2(Get, std::string(const std::string& url, const std::string& token));
    MOCK_METHOD3(Post, std::string(const std::string& url, const std::string& body, const std::string& token));
};
