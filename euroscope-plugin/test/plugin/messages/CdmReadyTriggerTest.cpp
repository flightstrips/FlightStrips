#include <gmock/gmock.h>
#include <gtest/gtest.h>

#include "messages/CdmReadyTrigger.h"

namespace {
    class MockTagFunctionInvoker final : public FlightStrips::messages::TagFunctionInvoker {
    public:
        MOCK_METHOD(bool, SelectActiveAircraft, (const std::string&), (override));
        MOCK_METHOD(
            void,
            InvokeTagFunction,
            (const std::string&, const std::string&, int, const std::string&, const std::string&, int),
            (override));
    };
}

TEST(CdmReadyTriggerTest, TriggerUsesCentralizedTopSkyReadyFunctionDetails) {
    FlightStrips::messages::CdmReadyTrigger trigger;
    MockTagFunctionInvoker invoker;

    testing::InSequence sequence;
    EXPECT_CALL(invoker, SelectActiveAircraft("SAS123")).WillOnce(testing::Return(true));
    EXPECT_CALL(invoker, InvokeTagFunction("", "CDM Plugin", 0, "", "CDM Plugin", 114));

    EXPECT_TRUE(trigger.Trigger(invoker, "SAS123"));
}

TEST(CdmReadyTriggerTest, TriggerRejectsEmptyCallsign) {
    FlightStrips::messages::CdmReadyTrigger trigger;
    MockTagFunctionInvoker invoker;

    EXPECT_CALL(invoker, SelectActiveAircraft).Times(0);
    EXPECT_CALL(invoker, InvokeTagFunction).Times(0);

    EXPECT_FALSE(trigger.Trigger(invoker, ""));
}

TEST(CdmReadyTriggerTest, TriggerRejectsUnknownAircraft) {
    FlightStrips::messages::CdmReadyTrigger trigger;
    MockTagFunctionInvoker invoker;

    EXPECT_CALL(invoker, SelectActiveAircraft("SAS123")).WillOnce(testing::Return(false));
    EXPECT_CALL(invoker, InvokeTagFunction).Times(0);

    EXPECT_FALSE(trigger.Trigger(invoker, "SAS123"));
}
