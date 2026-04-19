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
