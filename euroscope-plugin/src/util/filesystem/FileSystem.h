#pragma once

#include <filesystem>

namespace FlightStrips::filesystem {
    class FileSystem {
    public:
        explicit FileSystem(HINSTANCE dllInstance);

        std::filesystem::path GetLocalFilePath(const std::string& fileName);

        bool DeleteFileIfExists(const std::string& fileName);

        bool Rename(const std::string& oldFileName, const std::string& newFileName);

        bool DoesExist(const std::string& fileName);

    private:
        std::filesystem::path dllDirectory;

    };
}