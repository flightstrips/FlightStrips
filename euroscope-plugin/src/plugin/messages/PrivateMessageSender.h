#pragma once

#include <string>
#include <array>
#include <optional>
#include "euroscope/EuroScopePlugIn.h"

namespace FlightStrips::messages {
    struct PendingFrequencySubmission {
        HWND input = nullptr;
        ULONGLONG queuedAt = 0;
    };
    class PrivateMessageSender {
    public:
        static std::string SendFrequencyMessage(EuroScopePlugIn::CPlugIn& plugin, const std::string& callsign, const std::string& message, int frequencyKey, const std::string& primaryFrequency, PendingFrequencySubmission& submission);
        static std::array<INPUT, 2> NumpadMultiplyInputs();
        static std::optional<std::string> PollFrequencySubmission(const PendingFrequencySubmission& submission, ULONGLONG now = GetTickCount64());
        static void SendPrivateMessage(const std::string& callsign, const std::string& message);

    private:
        static bool SendCommandViaMessageInput(const std::string& command);
    };
}
