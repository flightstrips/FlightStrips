#pragma once
#include <string>
#include <memory>
#include "spdlog/spdlog.h"
#include "spdlog/sinks/rotating_file_sink.h"
#include "spdlog/sinks/basic_file_sink.h"

class Logger;

enum LogLevel {
    LOG_DEBUG,
    LOG_INFO,
    LOG_WARNING,
    LOG_ERROR,
    LOG_NONE,
};

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

    static void SetInstance(std::shared_ptr<Logger> instance) {
        s_instance = std::move(instance);
    }

    static std::shared_ptr<Logger> GetInstance() {
        return s_instance;
    }

    static void Debug(const std::string &message) {
        if (!s_instance) return;
        s_instance->m_logger->debug(message);
    }
    static void Info(const std::string &message) {
        if (!s_instance) return;
        s_instance->m_logger->info(message);
    }
    static void Warning(const std::string &message) {
        if (!s_instance) return;
        s_instance->m_logger->warn(message);
    }
    static void Error(const std::string &message) {
        if (!s_instance) return;
        s_instance->m_logger->error(message);
    }

    template<typename... Args>
    static void Debug(spdlog::format_string_t<Args...> fmt, Args &&... args) {
        if (!s_instance) return;
        s_instance->m_logger->debug(fmt, std::forward<Args>(args)...);
    }

    template<typename... Args>
    static void Info(spdlog::format_string_t<Args...> fmt, Args &&... args) {
        if (!s_instance) return;
        s_instance->m_logger->info(fmt, std::forward<Args>(args)...);
    }

    template<typename... Args>
    static void Warning(spdlog::format_string_t<Args...> fmt, Args &&... args) {
        if (!s_instance) return;
        s_instance->m_logger->warn(fmt, std::forward<Args>(args)...);
    }

    template<typename... Args>
    static void Error(spdlog::format_string_t<Args...> fmt, Args &&... args) {
        if (!s_instance) return;
        s_instance->m_logger->error(fmt, std::forward<Args>(args)...);
    }

    static std::shared_ptr<Logger> Init(const std::string &logPath, const LogLevel level) {
        struct EnableMakeShared : Logger {
            EnableMakeShared(const std::string &logPath, const LogLevel level) : Logger(logPath, level) {}
        };
        auto instance = std::make_shared<EnableMakeShared>(logPath, level);
        SetInstance(instance);
        return instance;
    }

    static void Shutdown() {
        s_instance.reset();
    }

private:
    Logger(const std::string &logPath, const LogLevel level) {
        m_logger = spdlog::rotating_logger_mt("logger", logPath, 1024 * 1024 * 5, 3);
        m_logger->set_level(ConvertToSpdlogLevel(level));
        m_logger->flush_on(spdlog::level::trace);
        m_logger->set_pattern("[%Y-%m-%d %H:%M:%S.%e] [%^%l%$] %v");
    }

    ~Logger() {
        if (m_logger) {
            m_logger->flush();
            m_logger.reset();
        }
    }

    static spdlog::level::level_enum ConvertToSpdlogLevel(const LogLevel level) {
        switch (level) {
            case LOG_DEBUG:
                return spdlog::level::debug;
            case LOG_INFO:
                return spdlog::level::info;
            case LOG_WARNING:
                return spdlog::level::warn;
            case LOG_ERROR:
                return spdlog::level::err;
            case LOG_NONE:
                return spdlog::level::off;
            default:
                return spdlog::level::info;
        }
    }

    static inline std::shared_ptr<Logger> s_instance;
    std::shared_ptr<spdlog::logger> m_logger;

};

