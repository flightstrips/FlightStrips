#include <gtest/gtest.h>
#include "plugin/VisibleControllers.h"

namespace {
    struct Controller {
        std::string callsign;
        double frequency = 0;
        int facility = 0;
        size_t index = 0;
        bool IsValid() const { return !callsign.empty(); }
        const char* GetCallsign() const { return callsign.c_str(); }
        double GetPrimaryFrequency() const { return frequency; }
        int GetFacility() const { return facility; }
    };
    auto read(const Controller& me, std::vector<Controller> peers) {
        for (size_t i = 0; i < peers.size(); ++i) peers[i].index = i;
        return FlightStrips::ReadVisibleControllers(me,
            [&] { return peers.empty() ? Controller{} : peers[0]; },
            [&](const Controller& c) { return c.index + 1 < peers.size() ? peers[c.index + 1] : Controller{}; });
    }
}

TEST(VisibleControllersTest, ReadsSelfAndNonPluginPeersWithoutPositionFileFiltering) {
    const Controller me{"EKDK_CTR", 136.555, 6};
    const auto result = read(me, {{"EKBI_APP", 127.580, 5}, me, {"EKBI_ATIS", 118.0, 4}, {"TRAINING_OBS", 199.998, 0}});
    ASSERT_TRUE(result.has_value());
    ASSERT_EQ(result->size(), 3u);
    EXPECT_EQ((*result)[0].callsign, "EKBI_APP");
    EXPECT_EQ((*result)[0].primary_frequency, "127.580");
    EXPECT_EQ((*result)[1].callsign, "EKDK_CTR");
    EXPECT_EQ((*result)[1].primary_frequency, "136.555");
    EXPECT_TRUE((*result)[2].observer);
}

TEST(VisibleControllersTest, ResamplesJoinDepartureAndFrequencyChangesWithoutCaching) {
    const Controller me{"EKDK_CTR", 136.555, 6};
    ASSERT_EQ(read(me, {})->size(), 1u);
    auto result = read(me, {{"EKBI_APP", 127.580, 5}});
    ASSERT_EQ(result->size(), 2u);
    result = read(me, {{"EKBI_APP", 119.005, 5}});
    EXPECT_EQ((*result)[0].primary_frequency, "119.005");
    EXPECT_EQ(read(me, {})->size(), 1u);
    EXPECT_FALSE(read({}, {{"EKBI_APP", 127.580, 5}}).has_value());
}
