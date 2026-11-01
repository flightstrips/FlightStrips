#include <gtest/gtest.h>

#include <array>
#include <filesystem>
#include <fstream>
#include <iterator>
#include <string>

namespace {
    auto GetPluginSourceRoot() -> std::filesystem::path {
        return std::filesystem::path(__FILE__).parent_path().parent_path().parent_path().parent_path() / "src" / "plugin";
    }

    auto ReadFile(const std::filesystem::path& path) -> std::string {
        std::ifstream input(path, std::ios::binary);
        return std::string(std::istreambuf_iterator<char>(input), std::istreambuf_iterator<char>());
    }

    auto IsSourceFile(const std::filesystem::path& path) -> bool {
        const auto extension = path.extension().string();
        return extension == ".cpp" || extension == ".h";
    }
}

TEST(LookupAuditTest, PluginSourceDoesNotUseDeprecatedEuroScopeLookups) {
    const auto sourceRoot = GetPluginSourceRoot();
    const std::array forbiddenAccessors = {
        "GetCorrelatedFlightPlan(",
        "GetCorrelatedRadarTarget(",
        "GetFPTrackPosition(",
        "IsReceived("
    };

    for (const auto& entry : std::filesystem::recursive_directory_iterator(sourceRoot)) {
        if (!entry.is_regular_file() || !IsSourceFile(entry.path())) continue;

        const auto contents = ReadFile(entry.path());
        for (const auto* accessor : forbiddenAccessors) {
            EXPECT_EQ(contents.find(accessor), std::string::npos)
                << accessor << " found in " << entry.path().string();
        }
    }
}

TEST(LookupAuditTest, RegistersExactlyOneReadOnlyAMANtag) {
    const auto plugin = ReadFile(GetPluginSourceRoot() / "plugin" / "FlightStripsPlugin.cpp");
    const auto firstType = plugin.find("RegisterTagItemType(\"AMAN");
    ASSERT_NE(firstType, std::string::npos);
    EXPECT_EQ(plugin.find("RegisterTagItemType(\"AMAN", firstType + 1), std::string::npos);
    EXPECT_EQ(plugin.find("RegisterTagItemFunction(\"AMAN"), std::string::npos);
}

TEST(LookupAuditTest, GroundTagReleaseUsesUnfilteredHandlerInsteadOfPluginLogic) {
    const auto sourceRoot = GetPluginSourceRoot();
    const auto plugin = ReadFile(sourceRoot / "plugin" / "FlightStripsPlugin.cpp");
    const auto bootstrap = ReadFile(sourceRoot / "bootstrap" / "InitializePlugin.cpp");
    const auto handler = ReadFile(sourceRoot / "handlers" / "GroundTagReleaseHandler.cpp");

    EXPECT_EQ(plugin.find("HandleRequestOnFrequency"), std::string::npos);
    EXPECT_EQ(plugin.find("ParseRequestOnFrequency"), std::string::npos);
    EXPECT_EQ(plugin.find("ShouldReleaseGroundTag"), std::string::npos);

    const auto callbackStart = plugin.find("void FlightStripsPlugin::OnFlightPlanControllerAssignedDataUpdate");
    ASSERT_NE(callbackStart, std::string::npos);
    const auto callbackEnd = plugin.find("void FlightStripsPlugin::OnFlightPlanFlightPlanDataUpdate", callbackStart);
    ASSERT_NE(callbackEnd, std::string::npos);
    const auto callback = plugin.substr(callbackStart, callbackEnd - callbackStart);
    const auto unfilteredDispatch = callback.find("m_unfilteredFlightPlanEventHandlers->ControllerFlightPlanDataEvent");
    const auto relevanceCheck = callback.find("IsRelevant(FlightPlan)");
    const auto filteredDispatch = callback.find("m_flightPlanEventHandlerCollection->ControllerFlightPlanDataEvent");
    ASSERT_NE(unfilteredDispatch, std::string::npos);
    ASSERT_NE(relevanceCheck, std::string::npos);
    ASSERT_NE(filteredDispatch, std::string::npos);
    EXPECT_LT(unfilteredDispatch, relevanceCheck);
    EXPECT_LT(relevanceCheck, filteredDispatch);

    const auto registrationStart = bootstrap.find("unfilteredFlightPlanEventHandlers->RegisterHandler(");
    ASSERT_NE(registrationStart, std::string::npos);
    const auto registrationEnd = bootstrap.find(";", registrationStart);
    ASSERT_NE(registrationEnd, std::string::npos);
    EXPECT_NE(bootstrap.substr(registrationStart, registrationEnd - registrationStart)
                  .find("std::make_shared<handlers::GroundTagReleaseHandler>"), std::string::npos);
    EXPECT_NE(handler.find("flightPlan.EndTracking()"), std::string::npos);
}

TEST(LookupAuditTest, AMANPluginCodeDoesNotReadLocalPredictionInputs) {
    const std::array forbiddenAccessors = {
        "GetRoute(", "GetPositionPredictions(", "GetScratchPadString(", "GetEstimatedLandingTime(",
        "albEvents", "EventSlot"
    };
    const std::array roots = {
        GetPluginSourceRoot() / "aman",
        GetPluginSourceRoot() / "tag_items" / "AMANGainLossHandler.cpp",
        GetPluginSourceRoot() / "tag_items" / "AMANGainLossHandler.h"
    };
    for (const auto& root : roots) {
        if (std::filesystem::is_regular_file(root)) {
            const auto contents = ReadFile(root);
            for (const auto* accessor : forbiddenAccessors) EXPECT_EQ(contents.find(accessor), std::string::npos);
            continue;
        }
        for (const auto& entry : std::filesystem::recursive_directory_iterator(root)) {
            if (!entry.is_regular_file() || !IsSourceFile(entry.path())) continue;
            const auto contents = ReadFile(entry.path());
            for (const auto* accessor : forbiddenAccessors) EXPECT_EQ(contents.find(accessor), std::string::npos);
        }
    }
}
