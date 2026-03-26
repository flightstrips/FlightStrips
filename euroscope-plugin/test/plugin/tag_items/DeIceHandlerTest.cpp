#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "tag_items/DeIceHandler.h"
#include "stands/StandService.h"
#include "stands/Stand.h"
#include "configuration/AppConfig.h"

using FlightStrips::TagItems::DeIceHandler;
using FlightStrips::stands::StandService;
using FlightStrips::stands::Stand;
using FlightStrips::configuration::AppConfig;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

static std::shared_ptr<AppConfig> MakeEmptyAppConfig() {
    return std::make_shared<AppConfig>("__nonexistent_app_config__.ini");
}

static std::shared_ptr<StandService> MakeEmptyStandService() {
    return std::make_shared<StandService>(std::vector<Stand>{});
}

// ---------------------------------------------------------------------------
// DeIceHandler construction
// ---------------------------------------------------------------------------

TEST(DeIceHandlerTest, ConstructWithEmptyDependencies_DoesNotThrow) {
    EXPECT_NO_THROW({
        DeIceHandler handler(MakeEmptyStandService(), MakeEmptyAppConfig());
    });
    EXPECT_EQ(DeIceHandler::DefaultDisplayColor(), RGB(212, 214, 7));
}

// ---------------------------------------------------------------------------
// FlightPlanDisconnectEvent — removes cached entry (no-op when never seen)
// ---------------------------------------------------------------------------

TEST(DeIceHandlerTest, FlightPlanDisconnectEvent_UnknownCallsign_DoesNotCrash) {
    DeIceHandler handler(MakeEmptyStandService(), MakeEmptyAppConfig());
    EuroScopePlugIn::CFlightPlan fp;
    EXPECT_NO_FATAL_FAILURE(handler.FlightPlanDisconnectEvent(fp));
}

// ---------------------------------------------------------------------------
// FlightPlanEvent and ControllerFlightPlanDataEvent — no-op overrides
// ---------------------------------------------------------------------------

TEST(DeIceHandlerTest, FlightPlanEvent_DoesNotCrash) {
    DeIceHandler handler(MakeEmptyStandService(), MakeEmptyAppConfig());
    EuroScopePlugIn::CFlightPlan fp;
    EXPECT_NO_FATAL_FAILURE(handler.FlightPlanEvent(fp));
}

TEST(DeIceHandlerTest, ControllerFlightPlanDataEvent_DoesNotCrash) {
    DeIceHandler handler(MakeEmptyStandService(), MakeEmptyAppConfig());
    EuroScopePlugIn::CFlightPlan fp;
    EXPECT_NO_FATAL_FAILURE(handler.ControllerFlightPlanDataEvent(fp, 0));
}

// ---------------------------------------------------------------------------
// Handle — fallback path (empty AppConfig → empty order → copies fallback)
//
// With an empty DeIceConfig (order is empty, fallback is ""), Handle iterates
// over zero actions and copies the fallback ("") to sItemString.
// The result is a null-terminated empty string at sItemString[0].
// ---------------------------------------------------------------------------

TEST(DeIceHandlerTest, Handle_EmptyConfig_WritesFallbackToBuffer) {
    DeIceHandler handler(MakeEmptyStandService(), MakeEmptyAppConfig());

    EuroScopePlugIn::CFlightPlan fp;
    EuroScopePlugIn::CRadarTarget rt;
    char sItemString[16] = "UNCHANGED";
    int colorCode = 0;
    COLORREF rgb = 0;
    double fontSize = 0.0;

    EXPECT_NO_FATAL_FAILURE(
        handler.Handle(fp, rt, 0, 0, sItemString, &colorCode, &rgb, &fontSize)
    );

    // Fallback is "" → first byte should be '\0' (empty string).
    EXPECT_EQ(sItemString[0], '\0');
    EXPECT_EQ(colorCode, 1);
    EXPECT_EQ(rgb, DeIceHandler::DefaultDisplayColor());
}

TEST(DeIceHandlerTest, Handle_CalledTwice_DoesNotCrash) {
    // Second call exercises the cache-hit path (callsign == "" both times,
    // so after the first call the empty-string entry is in the cache).
    DeIceHandler handler(MakeEmptyStandService(), MakeEmptyAppConfig());

    EuroScopePlugIn::CFlightPlan fp;
    EuroScopePlugIn::CRadarTarget rt;
    char buf[16] = {};
    int colorCode = 0;
    COLORREF rgb = 0;
    double fontSize = 0.0;

    handler.Handle(fp, rt, 0, 0, buf, &colorCode, &rgb, &fontSize);
    EXPECT_NO_FATAL_FAILURE(
        handler.Handle(fp, rt, 0, 0, buf, &colorCode, &rgb, &fontSize)
    );
}

TEST(DeIceHandlerTest, Handle_AfterDisconnect_DoesNotCrash) {
    // Disconnect followed by another Handle call should not crash.
    DeIceHandler handler(MakeEmptyStandService(), MakeEmptyAppConfig());

    EuroScopePlugIn::CFlightPlan fp;
    EuroScopePlugIn::CRadarTarget rt;
    char buf[16] = {};
    int colorCode = 0;
    COLORREF rgb = 0;
    double fontSize = 0.0;

    handler.Handle(fp, rt, 0, 0, buf, &colorCode, &rgb, &fontSize);
    handler.FlightPlanDisconnectEvent(fp);
    EXPECT_NO_FATAL_FAILURE(
        handler.Handle(fp, rt, 0, 0, buf, &colorCode, &rgb, &fontSize)
    );
}
