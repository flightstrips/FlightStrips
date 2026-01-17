
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

    bool FileSystem::DeleteFileIfExists(const std::string &fileName) {
        if (const auto filePath = GetLocalFilePath(fileName); std::filesystem::exists(filePath)) {
            return std::filesystem::remove(filePath);
        }

        return false;
    }

    bool FileSystem::Rename(const std::string &oldFileName, const std::string &newFileName) {
        const auto oldPath = GetLocalFilePath(oldFileName);
        const auto newPath = GetLocalFilePath(newFileName);

        if (!std::filesystem::exists(oldPath)) {
            return false;
        }

        try {
            std::filesystem::rename(oldPath, newPath);
            return true;
        } catch (...) {
            return false;
        }
    }

    bool FileSystem::DoesExist(const std::string &fileName) {
        const auto filePath = GetLocalFilePath(fileName);
        return std::filesystem::exists(filePath);
    }
}