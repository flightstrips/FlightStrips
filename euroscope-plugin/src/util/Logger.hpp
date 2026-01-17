#pragma once
#include <chrono>
#include <fstream>
#include <iostream>
#include <string>
#include <syncstream>
#include <utility>

class Logger;

enum LogLevel {
    LOG_DEBUG,
    LOG_INFO,
    LOG_WARNING,
    LOG_ERROR,
    LOG_NONE,
};

static Logger* log_instance;

class Logger {
public:
    static LogLevel GetLevelFromString(const std::string &logLevel) {
        if (logLevel == "DEBUG") {
            return LOG_DEBUG;
        }
        if (logLevel == "WARNING") {
            return LOG_WARNING;
        }
        if (logLevel == "ERROR") {
            return LOG_ERROR;
        }
        if (logLevel == "NONE") {
            return LOG_NONE;
        }
        return LOG_INFO;
    }

    static void Debug(const std::string &message) {
        if (!log_instance) return;
        log_instance->Log(message, LOG_DEBUG);
    }
    static void Info(const std::string &message) {
        if (!log_instance) return;
        log_instance->Log(message, LOG_INFO);
    }
    static void Warning(const std::string &message) {
        if (!log_instance) return;
        log_instance->Log(message, LOG_WARNING);
    }
    static void Error(const std::string &message) {
        if (!log_instance) return;
        log_instance->Log(message, LOG_ERROR);
    }

    template<class... _Types>
    static void Debug(const std::format_string<_Types...> fmt, _Types &&... args) {
        if (!log_instance) return;
        if (!log_instance->IsEnabled(LOG_DEBUG)) return;
        log_instance->Log(std::vformat(fmt.get(), std::make_format_args(args...)), LOG_DEBUG);
    }

    template<class... _Types>
    static void Info(const std::format_string<_Types...> fmt, _Types &&... args) {
        if (!log_instance) return;
        if (!log_instance->IsEnabled(LOG_INFO)) return;
        log_instance->Log(std::vformat(fmt.get(), std::make_format_args(args...)), LOG_INFO);
    }

    template<class... _Types>
    static void Warning(const std::format_string<_Types...> fmt, _Types &&... args) {
        if (!log_instance) return;
        if (!log_instance->IsEnabled(LOG_WARNING)) return;
        log_instance->Log(std::vformat(fmt.get(), std::make_format_args(args...)), LOG_WARNING);
    }

    template<class... _Types>
    static void Error(const std::format_string<_Types...> fmt, _Types &&... args) {
        if (!log_instance) return;
        if (!log_instance->IsEnabled(LOG_ERROR)) return;
        log_instance->Log(std::vformat(fmt.get(), std::make_format_args(args...)), LOG_ERROR);
    }


    static void Init(const std::string &logPath, const LogLevel level) { log_instance = new Logger(logPath, level); }
    static void Shutdown() { delete log_instance; }

private:
    Logger(std::string logPath, const LogLevel level) : LOG_PATH(std::move(logPath)), LEVEL(level), out(std::cout) {
        file.open(logPath, std::ofstream::out | std::ofstream::trunc);
        out = std::osyncstream(file);
    }

    ~Logger() {
        out.flush();
        file.close();
    }

    std::string LOG_PATH;
    LogLevel LEVEL;
    std::ofstream file = {};
    std::osyncstream out;

    [[nodiscard]] bool IsEnabled(const LogLevel level) const { return !(level < LEVEL || LOG_PATH.empty()); }

    void Log(const std::string &message, const LogLevel &level) {
        if (!IsEnabled(level)) return;

        const auto now = std::chrono::system_clock::now();
        const std::string formatted_time = std::format("{0:%F %T}", now);
        out << "[" << formatted_time << "] " << GetLogString(level) << ": " << message << std::endl;
    }

    static std::string GetLogString(const LogLevel &level) {
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

