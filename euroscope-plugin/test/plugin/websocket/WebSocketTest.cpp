#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "websocket/WebSocket.h"
#include <websocketpp/server.hpp>
#include <websocketpp/config/asio_no_tls.hpp>
#include <atomic>
#include <chrono>
#include <thread>

using namespace FlightStrips::websocket;

namespace {
void CheckClientProtocolNegotiation(bool negotiate) {
    websocketpp::server<websocketpp::config::asio> server;
    server.clear_access_channels(websocketpp::log::alevel::all);
    server.clear_error_channels(websocketpp::log::elevel::all);
    server.init_asio();
    std::atomic<bool> opened{false}, closed{false}, connected{false};
    server.set_validate_handler([&](websocketpp::connection_hdl hdl) {
        if (negotiate) {
            server.get_con_from_hdl(hdl)->select_subprotocol("flightstrips.euroscope.pb.v2");
        }
        return true;
    });
    server.set_open_handler([&](websocketpp::connection_hdl) { opened = true; });
    server.set_close_handler([&](websocketpp::connection_hdl) { closed = true; });
    server.listen(asio::ip::tcp::endpoint(asio::ip::address_v4::loopback(), 0));
    websocketpp::lib::error_code ec;
    const auto port = server.get_local_endpoint(ec).port();
    server.start_accept();
    std::thread serverThread([&] { server.run(); });
    {
        WebSocket client("ws://127.0.0.1:" + std::to_string(port),
                         [](const std::string&) {}, [&] { connected = true; });
        client.Connect();
        const auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(5);
        while (!(negotiate ? connected.load() : closed.load()) &&
               std::chrono::steady_clock::now() < deadline) {
            std::this_thread::sleep_for(std::chrono::milliseconds(10));
        }
        EXPECT_TRUE(opened.load());
        EXPECT_EQ(connected.load(), negotiate);
        if (!negotiate) { EXPECT_TRUE(closed.load()); }
        client.Disconnect();
    }
    server.stop_listening(ec);
    server.stop();
    serverThread.join();
}
}

TEST(WebSocketTest, ClientAcceptsNegotiatedRevisionTwoResponseHeader) {
    CheckClientProtocolNegotiation(true);
}

TEST(WebSocketTest, ClientRejectsMissingRevisionTwoResponseHeader) {
    CheckClientProtocolNegotiation(false);
}
using ::testing::HasSubstr;

TEST(WebSocketTest, FormatCloseLogMessage_IncludesRemoteAndLocalCloseDetails) {
    const auto message = detail::FormatCloseLogMessage(
        websocketpp::close::status::normal,
        "token expired",
        websocketpp::close::status::going_away,
        "Stopping",
        "The operation completed successfully"
    );

    EXPECT_THAT(message, HasSubstr("Connection to server closed"));
    EXPECT_THAT(message, HasSubstr("remote_code=1000"));
    EXPECT_THAT(message, HasSubstr("remote_reason=\"token expired\""));
    EXPECT_THAT(message, HasSubstr("local_code=1001"));
    EXPECT_THAT(message, HasSubstr("local_reason=\"Stopping\""));
    EXPECT_THAT(message, HasSubstr("transport_reason=\"The operation completed successfully\""));
}

TEST(WebSocketTest, FormatCloseLogMessage_UsesPlaceholdersWhenReasonsAreMissing) {
    const auto message = detail::FormatCloseLogMessage(
        websocketpp::close::status::no_status,
        "",
        websocketpp::close::status::normal,
        "",
        ""
    );

    EXPECT_THAT(message, HasSubstr("remote_reason=\"<none>\""));
    EXPECT_THAT(message, HasSubstr("local_reason=\"<none>\""));
}
