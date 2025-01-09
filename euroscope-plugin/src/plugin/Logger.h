//
// Created by fsr19 on 09/01/2025.
//

#pragma once
#include <chrono>
#include <fstream>
#include <string>

enum LogLevel {
    LOG_DEBUG,
    LOG_INFO,
    LOG_WARNING,
    LOG_ERROR,
    LOG_NONE,
};

class Logger {
public:
    static LogLevel LEVEL;
    static std::string LOG_PATH;

    static void Debug(const std::string &message) { WriteToFile(message, LOG_DEBUG); }
    static void Info(const std::string &message) { WriteToFile(message, LOG_INFO); }
    static void Warning(const std::string &message) { WriteToFile(message, LOG_WARNING); }
    static void Error(const std::string &message) { WriteToFile(message, LOG_ERROR); }
private:
    static void WriteToFile(const std::string& message, const LogLevel& level) {
        if (level < LEVEL || LOG_PATH.empty()) {
            return;
        }

        const auto now = std::chrono::system_clock::now();
        const std::string formatted_time = std::format("{0:%F %T}", now);
        std::ofstream file;
        file.open(LOG_PATH, std::ofstream::out | std::ofstream::app);

        file << "[" << formatted_time << "] " << GetLogString(level) << ": " << message << std::endl;
        file.close();
    }

    static std::string GetLogString(const LogLevel& level) {
        switch (level) {
            case LOG_DEBUG:
                return "DEBUG";
            case LOG_INFO:
                return "INFO";
            case LOG_WARNING:
                return "WARNING";
            case LOG_ERROR:
                return "ERROR";
            default:
                return "UNKNOWN";
        }
    }
};



