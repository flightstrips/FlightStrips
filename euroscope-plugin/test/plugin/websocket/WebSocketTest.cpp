#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "websocket/WebSocket.h"

using namespace FlightStrips::websocket;
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
