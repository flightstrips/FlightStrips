
#include "FileSystem.h"
#include <span>

namespace FlightStrips::filesystem {
    FileSystem::FileSystem(HINSTANCE dllInstance) {
        char path[MAX_PATH + 1] = {0};
        std::span<char, MAX_PATH + 1> span(path);
        GetModuleFileNameA(dllInstance, span.data(), span.size());
        dllDirectory = std::filesystem::path(span.data());
        dllDirectory.remove_filename();
    }

    std::filesystem::path FileSystem::GetLocalFilePath(const std::string &fileName) {
        return std::filesystem::path(dllDirectory).append(fileName);
    }
}