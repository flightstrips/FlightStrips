#include <gtest/gtest.h>
#include "websocket/MembershipState.h"

using namespace FlightStrips::websocket;
namespace wire=::flightstrips::euroscope::v1;

static wire::Envelope JoinedMembership() {
    wire::Envelope event;
    auto* c=event.mutable_context();c->set_instance_id("instance");c->set_transport_generation(7);
    c->set_membership_id("member");c->set_generation(8);c->set_session_id(42);
    c->set_airport("AAAA");c->set_environment("LIVE");c->set_master_epoch(3);
    event.mutable_session_info()->set_role("slave");return event;
}

TEST(MembershipAuthorityTest, OwnHumanCommandSurvivesElectionWithoutLoweringEpoch) {
    MembershipState state;auto joined=JoinedMembership();ASSERT_TRUE(state.Accept(joined,"instance",7,"LIVE"));
    auto command=joined;command.mutable_generate_squawk()->set_callsign("SAS123");
    command.mutable_context()->set_master_epoch(1);command.set_command_origin(wire::COMMAND_ORIGIN_HUMAN);command.set_execution_id("backend-command");
    EXPECT_TRUE(state.Accept(command,"instance",7,"LIVE"));
    EXPECT_EQ(state.members.at("member").context.master_epoch(),3);
    EXPECT_EQ(state.members.at("member").role,STATE_SLAVE);
}

TEST(MembershipAuthorityTest, AutomaticCommandAndSnapshotRequireCurrentEpoch) {
    MembershipState state;auto joined=JoinedMembership();ASSERT_TRUE(state.Accept(joined,"instance",7,"LIVE"));
    auto command=joined;command.mutable_generate_squawk()->set_callsign("SAS123");command.set_command_origin(wire::COMMAND_ORIGIN_AUTOMATIC);
    command.mutable_context()->set_master_epoch(1);EXPECT_FALSE(state.Accept(command,"instance",7,"LIVE"));
    command.mutable_context()->set_master_epoch(3);EXPECT_TRUE(state.Accept(command,"instance",7,"LIVE"));
    auto snapshot=joined;snapshot.mutable_context()->set_master_epoch(1);snapshot.mutable_backend_sync();snapshot.set_command_origin(wire::COMMAND_ORIGIN_HUMAN);
    EXPECT_FALSE(state.Accept(snapshot,"instance",7,"LIVE"));
}

TEST(MembershipAuthorityTest, HumanCommandCannotCrossMembershipGenerationOrReviveRemoval) {
    MembershipState state;auto joined=JoinedMembership();ASSERT_TRUE(state.Accept(joined,"instance",7,"LIVE"));
    auto command=joined;command.mutable_assigned_squawk()->set_callsign("SAS123");command.set_command_origin(wire::COMMAND_ORIGIN_HUMAN);
    command.mutable_context()->set_generation(9);EXPECT_FALSE(state.Accept(command,"instance",7,"LIVE"));
    command.mutable_context()->set_generation(8);command.mutable_context()->set_environment("SWEATBOX");EXPECT_FALSE(state.Accept(command,"instance",7,"LIVE"));
    auto removed=joined;removed.mutable_membership_removed();EXPECT_FALSE(state.Accept(removed,"instance",7,"LIVE"));
    command.mutable_context()->set_environment("LIVE");EXPECT_FALSE(state.Accept(command,"instance",7,"LIVE"));
    state.Reset();EXPECT_FALSE(state.Accept(command,"instance",7,"LIVE"));
}

TEST(MembershipAuthorityTest, AirportRetirementAndEpochChangeLeaveOtherMembershipUsable) {
    MembershipState state;
    auto a=JoinedMembership();
    auto b=a;
    b.mutable_context()->set_membership_id("other");
    b.mutable_context()->set_session_id(43);
    b.mutable_context()->set_airport("BBBB");
    b.mutable_context()->set_master_epoch(11);
    b.mutable_session_info()->set_role("master");
    ASSERT_TRUE(state.Accept(a,"instance",7,"LIVE"));
    ASSERT_TRUE(state.Accept(b,"instance",7,"LIVE"));
    auto inventory=b;
    inventory.mutable_backend_sync()->add_strips()->set_callsign("SAME123");
    ASSERT_TRUE(state.Accept(inventory,"instance",7,"LIVE"));
    auto election=a;
    election.mutable_context()->set_master_epoch(4);
    election.mutable_session_info()->set_role("master");
    ASSERT_TRUE(state.Accept(election,"instance",7,"LIVE"));
    auto removed=election;
    removed.mutable_membership_removed();
    EXPECT_FALSE(state.Accept(removed,"instance",7,"LIVE"));
    EXPECT_EQ(state.members.size(),1);
    EXPECT_EQ(state.members.at("other").context.master_epoch(),11);
    EXPECT_EQ(state.members.at("other").role,STATE_MASTER);
    EXPECT_TRUE(state.members.at("other").inventory.contains("SAME123"));
    auto command=b;
    command.mutable_assigned_squawk()->set_callsign("SAME123");
    command.set_command_origin(wire::COMMAND_ORIGIN_HUMAN);
    EXPECT_TRUE(state.Accept(command,"instance",7,"LIVE"));
    // Old queued A traffic cannot revive the retired membership, including
    // after a new membership for A is announced on the same physical socket.
    EXPECT_FALSE(state.Accept(a,"instance",7,"LIVE"));
    auto rejoin=election;
    rejoin.mutable_context()->set_membership_id("rejoined");
    rejoin.mutable_context()->set_generation(9);
    ASSERT_TRUE(state.Accept(rejoin,"instance",7,"LIVE"));
    EXPECT_FALSE(state.Accept(a,"instance",7,"LIVE"));
    EXPECT_TRUE(state.Accept(command,"instance",7,"LIVE"));
    EXPECT_EQ(state.members.size(),2);
}
