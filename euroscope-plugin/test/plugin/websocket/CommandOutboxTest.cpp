#include <gtest/gtest.h>

#include "websocket/CommandOutbox.h"

using FlightStrips::websocket::CommandOutbox;
namespace wire = flightstrips::euroscope::v1;

TEST(CommandOutboxTest, ReconnectResendsResultWithoutExecutingTwice) {
    CommandOutbox outbox;
    const std::string id = "11111111-1111-4111-8111-111111111111";
    EXPECT_FALSE(outbox.Begin("not-a-uuid"));
    ASSERT_TRUE(outbox.Begin(id));
    ASSERT_FALSE(outbox.Begin(id));

    const auto bytes = outbox.Complete(id, wire::CommandResultEvent::FAILED,
        wire::CommandResultEvent::UI_UNAVAILABLE, "message input unavailable", 42, 9, 3);
    ASSERT_FALSE(bytes.empty());
    wire::Envelope result;
    ASSERT_TRUE(result.ParseFromString(bytes));
    EXPECT_EQ(result.event_case(), wire::Envelope::kCommandResult);
    EXPECT_EQ(result.command_id(), id);
    EXPECT_EQ(result.command_result().command_id(), id);
    EXPECT_EQ(result.command_result().reason(), wire::CommandResultEvent::UI_UNAVAILABLE);
    EXPECT_EQ(result.session_id(), 42);
    EXPECT_EQ(result.owner_epoch(), 9);
    EXPECT_EQ(result.master_epoch(), 3);
    EXPECT_EQ(outbox.Pending(), std::vector<std::string>{bytes});

    // A reconnect reads pending bytes, while the process-wide seen set remains.
    ASSERT_FALSE(outbox.Begin(id));
    outbox.Acknowledge(id);
    EXPECT_TRUE(outbox.Pending().empty());
    EXPECT_FALSE(outbox.Begin(id));
}
