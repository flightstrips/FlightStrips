#include "aman/AMANGainLossStore.h"

#include <fstream>
#include <thread>

#include <gtest/gtest.h>

using FlightStrips::aman::AMANGainLossStore;

namespace {
    auto Event(const unsigned long long revision, const std::string& callsign = "SAS123") -> nlohmann::json {
        return {
            {"type", "aman_gain_loss"}, {"version", 1}, {"airport", "EKCH"}, {"revision", revision},
            {"generated_at", "2026-09-08T12:00:00Z"}, {"authoritative", true},
            {"values", {{{"flight_id", "flight-1"}, {"callsign", callsign},
                         {"gain_loss_seconds", 90}, {"reference_point", "ILS-22L-RUNWAY"},
                         {"target_time", "2026-09-08T12:10:00Z"}, {"predicted_time", "2026-09-08T12:11:30Z"},
                         {"data_status", "fresh"}}}}
        };
    }
}

TEST(AMANGainLossStoreTest, ReadsSharedGoGoldenFixture) {
    std::ifstream input(AMAN_GAIN_LOSS_FIXTURE_PATH);
    ASSERT_TRUE(input.good());
    nlohmann::json fixture;
    input >> fixture;

    AMANGainLossStore store;
    store.OnMessages({fixture});

    const auto snapshot = store.Snapshot();
    EXPECT_EQ(snapshot->version, 1);
    EXPECT_EQ(snapshot->revision, 42);
    ASSERT_TRUE(store.FindByCallsign(" sas123 ").has_value());
    EXPECT_EQ(store.FindByCallsign("SAS123")->seconds, 90);
    EXPECT_EQ(store.FindByCallsign("SAS123")->referencePoint, "ILS-22L-RUNWAY");
    EXPECT_EQ(store.FindByCallsign("SAS123")->targetTime, "2026-09-08T12:10:00.000Z");
    EXPECT_EQ(store.FindByCallsign("SAS123")->predictedTime, "2026-09-08T12:11:30.000Z");
    EXPECT_EQ(store.FindByCallsign("DAT456")->dataStatus, "stale");
}

TEST(AMANGainLossStoreTest, ReplacesAtomicallyAndIgnoresOldOrDuplicateRevisions) {
    AMANGainLossStore store;
    store.OnMessages({Event(2)});
    store.OnMessages({Event(2, "DUP2"), Event(1, "OLD1")});
    EXPECT_TRUE(store.FindByCallsign("SAS123").has_value());
    EXPECT_FALSE(store.FindByCallsign("DUP2").has_value());

    store.OnMessages({Event(3, "NEW123")});
    EXPECT_FALSE(store.FindByCallsign("SAS123").has_value());
    EXPECT_EQ(store.FindByCallsign("new123")->flightId, "flight-1");
}

TEST(AMANGainLossStoreTest, InvalidReplacementClearsValuesUntilANewerValidRevision) {
    AMANGainLossStore store;
    store.OnMessages({Event(2)});
    auto invalid = Event(3);
    invalid["values"][0]["data_status"] = "unknown";
    store.OnMessages({invalid});
    EXPECT_TRUE(store.Snapshot()->byFlightId.empty());
    EXPECT_EQ(store.Snapshot()->revision, 2);

    store.OnMessages({Event(2)});
    EXPECT_TRUE(store.Snapshot()->byFlightId.empty());
    store.OnMessages({Event(3)});
    EXPECT_TRUE(store.FindByCallsign("SAS123").has_value());
}

TEST(AMANGainLossStoreTest, ReadersNeverObservePartialReplacement) {
    AMANGainLossStore store;
    auto first = Event(1);
    auto second = Event(2, "SECOND");
    for (int index = 2; index < 200; ++index) {
        first["values"].push_back({{"flight_id", "old-" + std::to_string(index)}, {"callsign", "OLD" + std::to_string(index)}, {"gain_loss_seconds", index}, {"reference_point", "ILS-22L-RUNWAY"}, {"target_time", "2026-09-08T12:10:00Z"}, {"predicted_time", "2026-09-08T12:11:30Z"}, {"data_status", "fresh"}});
        second["values"].push_back({{"flight_id", "new-" + std::to_string(index)}, {"callsign", "NEW" + std::to_string(index)}, {"gain_loss_seconds", index}, {"reference_point", "ILS-22L-RUNWAY"}, {"target_time", "2026-09-08T12:10:00Z"}, {"predicted_time", "2026-09-08T12:11:30Z"}, {"data_status", "fresh"}});
    }
    store.OnMessages({first});

    std::atomic<bool> done = false;
    std::thread writer([&] { store.OnMessages({second}); done = true; });
    while (!done) {
        const auto snapshot = store.Snapshot();
        EXPECT_TRUE(snapshot->revision == 1 || snapshot->revision == 2);
        EXPECT_EQ(snapshot->byFlightId.size(), 199);
    }
    writer.join();
}

TEST(AMANGainLossStoreTest, ReconnectHidesOldValuesAndAcceptsTheNewConnectionRevision) {
    AMANGainLossStore store;
    store.OnMessages({Event(42)});

    store.Online();
    EXPECT_FALSE(store.Snapshot()->hasRevision);
    EXPECT_FALSE(store.Snapshot()->authoritative);
    EXPECT_TRUE(store.Snapshot()->byFlightId.empty());

    store.OnMessages({Event(42, "NEW123")});
    EXPECT_TRUE(store.Snapshot()->hasRevision);
    EXPECT_TRUE(store.FindByCallsign("NEW123").has_value());
}

TEST(AMANGainLossStoreTest, SameRevisionAuthorityTransitionReplacesTheSnapshot) {
    AMANGainLossStore store;
    store.OnMessages({Event(42)});

    auto blocked = Event(42);
    blocked["authoritative"] = false;
    store.OnMessages({blocked});

    EXPECT_FALSE(store.Snapshot()->authoritative);
    EXPECT_TRUE(store.FindByCallsign("SAS123").has_value());
}

TEST(AMANGainLossStoreTest, RejectsPartialOrUnknownPresentationContract) {
    AMANGainLossStore store;
    auto partial = Event(1);
    partial["values"][0]["target_time"] = nullptr;
    store.OnMessages({partial});
    EXPECT_TRUE(store.Snapshot()->byFlightId.empty());

    auto unknownVersion = Event(1);
    unknownVersion["version"] = 2;
    store.OnMessages({unknownVersion});
    EXPECT_TRUE(store.Snapshot()->byFlightId.empty());
}
