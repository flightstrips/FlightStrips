#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "websocket/WebSocket.h"
#include <websocketpp/server.hpp>
#include <websocketpp/config/asio_no_tls.hpp>
#include <future>
#include <thread>

using namespace FlightStrips::websocket;
using ::testing::HasSubstr;

TEST(WebSocketTest, BinaryDefaultAndExplicitTextFramesRoundTripOnLoopback) {
    for (const bool text : {false, true}) {
        websocketpp::server<websocketpp::config::asio> server;
        server.clear_access_channels(websocketpp::log::alevel::all);
        server.clear_error_channels(websocketpp::log::elevel::all);
        server.init_asio();
        std::promise<websocketpp::frame::opcode::value> opcode;
        std::promise<std::string> received;
        server.set_message_handler([&](websocketpp::connection_hdl connection, auto message) {
            opcode.set_value(message->get_opcode());
            websocketpp::lib::error_code error;
            server.send(connection, message->get_payload(), message->get_opcode(), error);
        });
        server.listen(asio::ip::tcp::endpoint(asio::ip::address_v4::loopback(), 0));
        asio::error_code endpointError;
        const auto port = server.get_local_endpoint(endpointError).port();
        ASSERT_FALSE(endpointError);
        server.start_accept();
        std::thread worker([&] { server.run(); });
        auto opcodeResult = opcode.get_future();
        auto receivedResult = received.get_future();
        std::unique_ptr<WebSocket> socket;
        const auto receive = [&](const std::string& payload) { received.set_value(payload); };
        const auto open = [&] { socket->Send(text ? "{\"type\":\"ping\"}" : std::string("\0\1protobuf", 10)); };
        const auto url = "ws://127.0.0.1:" + std::to_string(port) + "/ws";
        if (text) socket = std::make_unique<WebSocket>(url, receive, open, true);
        else socket = std::make_unique<WebSocket>(url, receive, open);
        socket->Connect();
        const auto opcodeReady = opcodeResult.wait_for(std::chrono::seconds(5));
        const auto receivedReady = receivedResult.wait_for(std::chrono::seconds(5));
        EXPECT_EQ(opcodeReady, std::future_status::ready);
        EXPECT_EQ(receivedReady, std::future_status::ready);
        if (opcodeReady == std::future_status::ready) EXPECT_EQ(opcodeResult.get(), text ? websocketpp::frame::opcode::text : websocketpp::frame::opcode::binary);
        if (receivedReady == std::future_status::ready) EXPECT_EQ(receivedResult.get(), text ? "{\"type\":\"ping\"}" : std::string("\0\1protobuf", 10));
        socket.reset(); server.stop(); worker.join();
    }
}

TEST(WebSocketTest, RetiredOpenCallbackCannotPublishStatusOverNewConnection) {
    websocketpp::server<websocketpp::config::asio> server;
    server.clear_access_channels(websocketpp::log::alevel::all);
    server.clear_error_channels(websocketpp::log::elevel::all);
    server.init_asio();
    server.listen(asio::ip::tcp::endpoint(asio::ip::address_v4::loopback(), 0));
    asio::error_code error;
    const auto port = server.get_local_endpoint(error).port(); ASSERT_FALSE(error);
    server.start_accept();
    std::thread worker([&] { server.run(); });
    std::promise<void> firstOpen, releaseFirst;
    std::promise<WebSocketStatus> secondOpenStatus;
    auto firstReady = firstOpen.get_future(); auto release = releaseFirst.get_future();
    auto secondReady = secondOpenStatus.get_future();
    int opens = 0;
    std::unique_ptr<WebSocket> socket;
    socket = std::make_unique<WebSocket>("ws://127.0.0.1:" + std::to_string(port) + "/ws",
        [](const std::string&) {}, [&] {
            if (++opens == 1) { firstOpen.set_value(); release.wait(); }
            else if (opens == 2) secondOpenStatus.set_value(socket->GetStatus());
        });
    socket->Connect();
    const auto entered = firstReady.wait_for(std::chrono::seconds(5));
    EXPECT_EQ(entered, std::future_status::ready);
    if (entered == std::future_status::ready) {
        socket->Disconnect();
        EXPECT_EQ(socket->GetStatus(), WEBSOCKET_STATUS_DISCONNECTED);
        socket->Connect(); // The old callback must not overwrite this CONNECTING state.
        EXPECT_EQ(socket->GetStatus(), WEBSOCKET_STATUS_CONNECTING);
    }
    releaseFirst.set_value();
    if (entered == std::future_status::ready) {
        const auto reopened = secondReady.wait_for(std::chrono::seconds(5));
        EXPECT_EQ(reopened, std::future_status::ready);
        if (reopened == std::future_status::ready) EXPECT_EQ(secondReady.get(), WEBSOCKET_STATUS_CONNECTING);
    }
    socket.reset(); server.stop(); worker.join();
}

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
