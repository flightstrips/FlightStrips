#pragma once

#include <exception>
#include <functional>
#include <string>
#include <utility>

namespace FlightStrips::exceptions {
    struct ExceptionDetails {
        std::string message;
        bool isKnown;
    };

    [[nodiscard]] ExceptionDetails GetExceptionDetails(std::exception_ptr exception = std::current_exception()) noexcept;
    void LogException(const std::string& context, const ExceptionDetails& details) noexcept;
    void LogCurrentException(const std::string& context) noexcept;
    void InstallCrashHandlers(const std::string& component) noexcept;

    template <typename Func, typename OnError>
    void RunGuarded(const std::string& context, Func&& func, OnError&& onError) noexcept {
        try {
            std::invoke(std::forward<Func>(func));
        } catch (...) {
            const auto details = GetExceptionDetails();
            LogException(context, details);

            try {
                std::invoke(std::forward<OnError>(onError), details);
            } catch (...) {
                LogCurrentException("FlightStrips::exceptions::RunGuarded");
            }
        }
    }

    template <typename Func>
    void RunGuarded(const std::string& context, Func&& func) noexcept {
        RunGuarded(context, std::forward<Func>(func), [](const ExceptionDetails&) noexcept {
        });
    }

    template <typename T, typename Func, typename OnError>
    T RunGuardedOr(const std::string& context, T fallback, Func&& func, OnError&& onError) noexcept {
        try {
            return std::invoke(std::forward<Func>(func));
        } catch (...) {
            const auto details = GetExceptionDetails();
            LogException(context, details);

            try {
                std::invoke(std::forward<OnError>(onError), details);
            } catch (...) {
                LogCurrentException("FlightStrips::exceptions::RunGuardedOr");
            }

            return fallback;
        }
    }

    template <typename T, typename Func>
    T RunGuardedOr(const std::string& context, T fallback, Func&& func) noexcept {
        return RunGuardedOr<T>(context, fallback, std::forward<Func>(func), [](const ExceptionDetails&) noexcept {
        });
    }
}
