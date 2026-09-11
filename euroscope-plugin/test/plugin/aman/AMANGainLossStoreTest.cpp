#include "aman/AMANGainLossStore.h"

#include <thread>

#include <gtest/gtest.h>

using FlightStrips::aman::AMANGainLossStore;

namespace {
    namespace wire = flightstrips::euroscope::v1;

    auto Event(const unsigned long long revision, const std::string& callsign = "SAS123") -> wire::Envelope {
        wire::Envelope envelope;
        auto* event = envelope.mutable_aman_gain_loss();
        event->set_version(1);
        event->set_airport("EKCH");
        event->set_revision(revision);
        event->set_generated_at("2026-09-08T12:00:00Z");
        event->set_authoritative(true);
        auto* value = event->add_values();
        value->set_flight_id("flight-1");
        value->set_callsign(callsign);
        value->set_gain_loss_seconds(90);
        value->set_reference_point("ILS-22L-RUNWAY");
        value->set_target_time("2026-09-08T12:10:00Z");
        value->set_predicted_time("2026-09-08T12:11:30Z");
        value->set_data_status("fresh");
        return envelope;
    }

    auto Bytes(const wire::Envelope& envelope) -> std::string {
        return envelope.SerializeAsString();
    }
}

TEST(AMANGainLossStoreTest, ReadsProtobufReplacement) {
    AMANGainLossStore store;
    store.OnMessages({Bytes(Event(42))});

    const auto snapshot = store.Snapshot();
    EXPECT_EQ(snapshot->version, 1);
    EXPECT_EQ(snapshot->revision, 42);
    ASSERT_TRUE(store.FindByCallsign(" sas123 ").has_value());
    EXPECT_EQ(store.FindByCallsign("SAS123")->seconds, 90);
    EXPECT_EQ(store.FindByCallsign("SAS123")->referencePoint, "ILS-22L-RUNWAY");
    EXPECT_EQ(store.FindByCallsign("SAS123")->targetTime, "2026-09-08T12:10:00Z");
    EXPECT_EQ(store.FindByCallsign("SAS123")->predictedTime, "2026-09-08T12:11:30Z");
}

TEST(AMANGainLossStoreTest, ReplacesAtomicallyAndIgnoresOldOrDuplicateRevisions) {
    AMANGainLossStore store;
    store.OnMessages({Bytes(Event(2))});
    store.OnMessages({Bytes(Event(2, "DUP2")), Bytes(Event(1, "OLD1"))});
    EXPECT_TRUE(store.FindByCallsign("SAS123").has_value());
    EXPECT_FALSE(store.FindByCallsign("DUP2").has_value());

    store.OnMessages({Bytes(Event(3, "NEW123"))});
    EXPECT_FALSE(store.FindByCallsign("SAS123").has_value());
    EXPECT_EQ(store.FindByCallsign("new123")->flightId, "flight-1");
}

TEST(AMANGainLossStoreTest, InvalidReplacementClearsValuesUntilANewerValidRevision) {
    AMANGainLossStore store;
    store.OnMessages({Bytes(Event(2))});
    auto invalid = Event(3);
    invalid.mutable_aman_gain_loss()->mutable_values(0)->set_data_status("unknown");
    store.OnMessages({Bytes(invalid)});
    EXPECT_TRUE(store.Snapshot()->byFlightId.empty());
    EXPECT_EQ(store.Snapshot()->revision, 2);

    store.OnMessages({Bytes(Event(2))});
    EXPECT_TRUE(store.Snapshot()->byFlightId.empty());
    store.OnMessages({Bytes(Event(3))});
    EXPECT_TRUE(store.FindByCallsign("SAS123").has_value());
}

TEST(AMANGainLossStoreTest, ReadersNeverObservePartialReplacement) {
    AMANGainLossStore store;
    auto first = Event(1);
    auto second = Event(2, "SECOND");
    for (int index = 2; index < 200; ++index) {
        for (auto* envelope : {&first, &second}) {
            auto* value = envelope->mutable_aman_gain_loss()->add_values();
            const auto prefix = envelope == &first ? "old-" : "new-";
            const auto callsignPrefix = envelope == &first ? "OLD" : "NEW";
            value->set_flight_id(prefix + std::to_string(index));
            value->set_callsign(callsignPrefix + std::to_string(index));
            value->set_gain_loss_seconds(index);
            value->set_reference_point("ILS-22L-RUNWAY");
            value->set_target_time("2026-09-08T12:10:00Z");
            value->set_predicted_time("2026-09-08T12:11:30Z");
            value->set_data_status("fresh");
        }
    }
    store.OnMessages({Bytes(first)});

    std::atomic<bool> done = false;
    std::thread writer([&] { store.OnMessages({Bytes(second)}); done = true; });
    while (!done) {
        const auto snapshot = store.Snapshot();
        EXPECT_TRUE(snapshot->revision == 1 || snapshot->revision == 2);
        EXPECT_EQ(snapshot->byFlightId.size(), 199);
    }
    writer.join();
}

TEST(AMANGainLossStoreTest, ReconnectHidesOldValuesAndAcceptsTheNewConnectionRevision) {
    AMANGainLossStore store;
    store.OnMessages({Bytes(Event(42))});

    store.Online();
    EXPECT_FALSE(store.Snapshot()->hasRevision);
    EXPECT_FALSE(store.Snapshot()->authoritative);
    EXPECT_TRUE(store.Snapshot()->byFlightId.empty());

    store.OnMessages({Bytes(Event(42, "NEW123"))});
    EXPECT_TRUE(store.Snapshot()->hasRevision);
    EXPECT_TRUE(store.FindByCallsign("NEW123").has_value());
}

TEST(AMANGainLossStoreTest, SameRevisionAuthorityTransitionReplacesTheSnapshot) {
    AMANGainLossStore store;
    store.OnMessages({Bytes(Event(42))});

    auto blocked = Event(42);
    blocked.mutable_aman_gain_loss()->set_authoritative(false);
    store.OnMessages({Bytes(blocked)});

    EXPECT_FALSE(store.Snapshot()->authoritative);
    EXPECT_TRUE(store.FindByCallsign("SAS123").has_value());
}

TEST(AMANGainLossStoreTest, RejectsPartialOrUnknownPresentationContract) {
    AMANGainLossStore store;
    auto partial = Event(1);
    partial.mutable_aman_gain_loss()->mutable_values(0)->clear_target_time();
    store.OnMessages({Bytes(partial)});
    EXPECT_TRUE(store.Snapshot()->byFlightId.empty());

    auto unknownVersion = Event(1);
    unknownVersion.mutable_aman_gain_loss()->set_version(2);
    store.OnMessages({Bytes(unknownVersion)});
    EXPECT_TRUE(store.Snapshot()->byFlightId.empty());
}
