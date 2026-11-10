#include <gtest/gtest.h>
#include "handlers/AircraftSelectionHandler.h"

using FlightStrips::handlers::AircraftSelectionHandler;
using Context = AircraftSelectionHandler::Context;

namespace {
    Context Membership(const std::string& id, uint64_t generation = 1) {
        Context context;
        context.set_instance_id("own-instance");
        context.set_membership_id(id);
        context.set_session_id(1);
        context.set_generation(generation);
        context.set_transport_generation(1);
        return context;
    }
}

TEST(AircraftSelectionHandlerTest, PublishesChangesAndDeselectionOnceForEachMembership) {
    std::string selected = "SAS123";
    std::vector<Context> contexts{Membership("master"), Membership("slave"), Membership("observer")};
    std::vector<std::pair<std::string, std::string>> sent;
    int reads = 0;
    AircraftSelectionHandler handler([&] { ++reads; return selected; }, [&] { return contexts; },
        [&](const auto& callsign, const auto& context) {
            sent.emplace_back(context.membership_id(), callsign); return true;
        });
    handler.OnTimer(1);
    ASSERT_EQ(sent.size(), 3);
    handler.OnTimer(2);
    EXPECT_EQ(sent.size(), 3);
    selected = "UNRELATED";
    handler.OnTimer(3);
    ASSERT_EQ(sent.size(), 6);
    EXPECT_EQ(sent.back().second, "UNRELATED");
    selected.clear();
    handler.OnTimer(4);
    ASSERT_EQ(sent.size(), 9);
    EXPECT_EQ(sent.back().second, "");
    handler.OnTimer(5);
    EXPECT_EQ(sent.size(), 9);
    EXPECT_EQ(reads, 5);
}

TEST(AircraftSelectionHandlerTest, RepublishesOnAttachmentAndReconnectButNotElection) {
    std::vector<Context> contexts;
    std::vector<Context> sent;
    AircraftSelectionHandler handler([] { return "SAS123"; }, [&] { return contexts; },
        [&](const auto&, const auto& context) { sent.push_back(context); return true; });
    handler.OnTimer(1);
    contexts = {Membership("a")};
    handler.OnTimer(2);
    ASSERT_EQ(sent.size(), 1);
    contexts[0].set_master_epoch(9);
    handler.OnTimer(3);
    EXPECT_EQ(sent.size(), 1);
    contexts.push_back(Membership("b"));
    handler.OnTimer(4);
    EXPECT_EQ(sent.size(), 2);
    contexts.clear();
    handler.OnTimer(5);
    contexts = {Membership("a", 2)};
    handler.OnTimer(6);
    ASSERT_EQ(sent.size(), 3);
    EXPECT_EQ(sent.back().generation(), 2);
}

TEST(AircraftSelectionHandlerTest, RetriesUnsentInitialDeselection) {
    bool available = false;
    int attempts = 0;
    AircraftSelectionHandler handler([] { return ""; }, [] { return std::vector{Membership("a")}; },
        [&](const auto& callsign, const auto&) { ++attempts; EXPECT_TRUE(callsign.empty()); return available; });
    handler.OnTimer(1);
    available = true;
    handler.OnTimer(2);
    handler.OnTimer(3);
    EXPECT_EQ(attempts, 2);
}
