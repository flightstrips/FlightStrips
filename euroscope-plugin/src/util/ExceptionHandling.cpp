#include "ExceptionHandling.h"

#include <cstdlib>
#include <format>

#include "Logger.hpp"

namespace FlightStrips::exceptions {
    namespace {
        std::string& CrashHandlerComponent() {
            static std::string component = "FlightStrips";
            return component;
        }

        void EmitFallbackLog(const std::string& message) noexcept {
            try {
                if (Logger::GetInstance()) {
                    Logger::Error(message);
                } else {
                    OutputDebugStringA((message + "\n").c_str());
                }
            } catch (...) {
                OutputDebugStringA("FlightStrips: failed to emit crash log.\n");
            }
        }

        LONG WINAPI UnhandledStructuredExceptionLogger(EXCEPTION_POINTERS* exceptionPointers) {
            try {
                const auto code = exceptionPointers != nullptr && exceptionPointers->ExceptionRecord != nullptr
                                      ? static_cast<unsigned long>(exceptionPointers->ExceptionRecord->ExceptionCode)
                                      : 0UL;
                EmitFallbackLog(std::format("Unhandled structured exception in {}: 0x{:08X}",
                                            CrashHandlerComponent(), code));
            } catch (...) {
                OutputDebugStringA("FlightStrips: unhandled structured exception.\n");
            }

            return EXCEPTION_CONTINUE_SEARCH;
        }

        [[noreturn]] void TerminateLogger() noexcept {
            try {
                LogException(std::format("{}::std::terminate", CrashHandlerComponent()),
                             GetExceptionDetails(std::current_exception()));
            } catch (...) {
                OutputDebugStringA("FlightStrips: std::terminate invoked.\n");
            }

            std::abort();
        }
    }

    ExceptionDetails GetExceptionDetails(std::exception_ptr exception) noexcept {
        if (exception == nullptr) {
            return {"No active exception", true};
        }

        try {
            std::rethrow_exception(exception);
        } catch (const std::exception& e) {
            return {e.what(), true};
        } catch (const std::string& e) {
            return {e, true};
        } catch (const char* e) {
            return {e == nullptr ? "null C-string exception" : e, true};
        } catch (...) {
            return {"Unknown exception", false};
        }
    }

    void LogException(const std::string& context, const ExceptionDetails& details) noexcept {
        if (details.isKnown) {
            EmitFallbackLog(std::format("Exception in {}: {}", context, details.message));
            return;
        }

        EmitFallbackLog(std::format("Unknown exception in {}", context));
    }

    void LogCurrentException(const std::string& context) noexcept {
        LogException(context, GetExceptionDetails());
    }

    void InstallCrashHandlers(const std::string& component) noexcept {
        try {
            CrashHandlerComponent() = component;
            std::set_terminate(TerminateLogger);
            SetUnhandledExceptionFilter(UnhandledStructuredExceptionLogger);
        } catch (...) {
            OutputDebugStringA("FlightStrips: failed to install crash handlers.\n");
        }
    }
}
