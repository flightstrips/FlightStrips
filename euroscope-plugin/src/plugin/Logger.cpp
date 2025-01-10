//
// Created by fsr19 on 09/01/2025.
//

#include "Logger.h"

LogLevel Logger::LEVEL = LOG_NONE;
std::string Logger::LOG_PATH;

void Logger::SetLevelFromString(const std::string &logLevel) {
    if (logLevel == "DEBUG") {
        LEVEL = LOG_DEBUG;
    } else if (logLevel == "WARNING") {
        LEVEL = LOG_WARNING;
    } else if (logLevel == "ERROR") {
        LEVEL = LOG_ERROR;
    } else if (logLevel == "NONE") {
        LEVEL = LOG_NONE;
    } else {
        LEVEL = LOG_INFO;
    }
}
