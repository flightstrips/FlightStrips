#include "PrivateMessageSender.h"

#include "Logger.hpp"

#include <Windows.h>
#include <cmath>
#include <cstdlib>

namespace FlightStrips::messages {
    namespace {
        constexpr int kMessageInputControlId = 1003;

        struct MainWindowSearchContext {
            DWORD processId = 0;
            HWND mainWindow = nullptr;
        };

        auto ReadWindowText(HWND hwnd) -> std::string {
            const int textLength = GetWindowTextLengthA(hwnd);
            if (textLength <= 0) {
                return "";
            }

            std::string text(static_cast<size_t>(textLength) + 1, '\0');
            const int copied = GetWindowTextA(hwnd, text.data(), textLength + 1);
            if (copied <= 0) {
                return "";
            }

            text.resize(static_cast<size_t>(copied));
            return text;
        }

        auto ReadWindowClass(HWND hwnd) -> std::string {
            char className[64] = {};
            const int copied = GetClassNameA(hwnd, className, static_cast<int>(std::size(className)));
            if (copied <= 0) {
                return "";
            }

            return {className, static_cast<size_t>(copied)};
        }

        BOOL CALLBACK FindMainWindowProc(HWND hwnd, LPARAM lParam) {
            auto* context = reinterpret_cast<MainWindowSearchContext*>(lParam);

            DWORD windowProcessId = 0;
            GetWindowThreadProcessId(hwnd, &windowProcessId);
            if (windowProcessId != context->processId || !IsWindowVisible(hwnd)) {
                return TRUE;
            }

            const auto windowClass = ReadWindowClass(hwnd);
            const auto title = ReadWindowText(hwnd);
            if (windowClass == "#32770" && title.find("EuroScope") != std::string::npos) {
                context->mainWindow = hwnd;
                return FALSE;
            }

            return TRUE;
        }

        auto FindEuroScopeMainWindow() -> HWND {
            MainWindowSearchContext context{};
            context.processId = GetCurrentProcessId();
            EnumWindows(FindMainWindowProc, reinterpret_cast<LPARAM>(&context));
            return context.mainWindow;
        }

        void PostReturnKey(HWND hwnd) {
            const UINT scanCode = MapVirtualKeyA(VK_RETURN, MAPVK_VK_TO_VSC);
            const LPARAM keyDown = 1 | (static_cast<LPARAM>(scanCode) << 16);
            const LPARAM keyUp = keyDown | (1L << 30) | (1L << 31);

            PostMessageA(hwnd, WM_KEYDOWN, VK_RETURN, keyDown);
            PostMessageA(hwnd, WM_CHAR, '\r', keyDown);
            PostMessageA(hwnd, WM_KEYUP, VK_RETURN, keyUp);
        }
    }

    std::array<INPUT, 2> PrivateMessageSender::NumpadMultiplyInputs() {
        std::array<INPUT, 2> keys{};
        for (auto& key : keys) {
            key.type = INPUT_KEYBOARD;
            key.ki.wScan = static_cast<WORD>(MapVirtualKeyA(VK_MULTIPLY, MAPVK_VK_TO_VSC));
            key.ki.dwFlags = KEYEVENTF_SCANCODE;
        }
        keys[1].ki.dwFlags |= KEYEVENTF_KEYUP;
        return keys;
    }

    std::optional<std::string> PrivateMessageSender::PollFrequencySubmission(const PendingFrequencySubmission& submission, ULONGLONG now) {
        if (!IsWindow(submission.input)) return "EuroScope command input closed before submission was confirmed";
        if (ReadWindowText(submission.input).empty()) return std::string{};
        if (now - submission.queuedAt >= 2000) return "Numpad Multiply did not consume the command input; check EuroScope before retrying";
        return std::nullopt;
    }

    void PrivateMessageSender::SendPrivateMessage(const std::string& callsign, const std::string& message) {
        const std::string command = ".msg " + callsign + " " + message;
        if (SendCommandViaMessageInput(command)) {
            return;
        }

        Logger::Warning("PrivateMessageSender: failed to send PM via EuroScope message input");
    }

    std::string PrivateMessageSender::SendFrequencyMessage(EuroScopePlugIn::CPlugIn& plugin,
        const std::string& callsign, const std::string& message, int frequencyKey, const std::string& primaryFrequency, PendingFrequencySubmission& submission) {
        if (callsign.empty() || message.empty() || message.size() > 1000 || message.front() == '.' ||
            message.find_first_of("\r\n") != std::string::npos || message.find('\0') != std::string::npos) {
            return "Invalid frequency message";
        }
        if (frequencyKey != VK_MULTIPLY) return "FREQ requires the numpad Multiply key";
        const auto me = plugin.ControllerMyself();
        if (!me.IsValid() || std::abs(me.GetPrimaryFrequency() - std::atof(primaryFrequency.c_str())) > 0.0005) {
            return "Primary frequency changed; reopen the text menu";
        }
        const auto flightPlan = plugin.FlightPlanSelect(callsign.c_str());
        if (!flightPlan.IsValid() || callsign != flightPlan.GetCallsign()) return "Aircraft is no longer available";
        const auto mainWindow = FindEuroScopeMainWindow();
        const auto input = mainWindow ? GetDlgItem(mainWindow, kMessageInputControlId) : nullptr;
        if (!input || ReadWindowClass(input) != "Edit") return "EuroScope command input is unavailable";
        if (!ReadWindowText(input).empty()) return "EuroScope command input is busy; finish or clear it first";

        // Native input must reach EuroScope's normal accelerator/message loop.
        // Direct SendMessage calls bypass that processing even with the right VK.
        if (GetWindowThreadProcessId(mainWindow, nullptr) != GetCurrentThreadId())
            return "Frequency submission must run on the EuroScope UI thread";
        for (const auto modifier : {VK_SHIFT, VK_CONTROL, VK_MENU, VK_LWIN, VK_RWIN}) {
            if ((GetAsyncKeyState(modifier) & 0x8000) != 0) return "Release modifier keys before sending a frequency message";
        }
        const auto foregroundThread = GetWindowThreadProcessId(GetForegroundWindow(), nullptr);
        const auto currentThread = GetCurrentThreadId();
        const bool attached = foregroundThread != 0 && foregroundThread != currentThread &&
            AttachThreadInput(currentThread, foregroundThread, TRUE);
        if (IsIconic(mainWindow)) ShowWindow(mainWindow, SW_RESTORE);
        SetForegroundWindow(mainWindow);
        SetFocus(input);
        if (attached) AttachThreadInput(currentThread, foregroundThread, FALSE);
        if (GetForegroundWindow() != mainWindow || GetFocus() != input)
            return "Could not focus EuroScope; activate its main window and retry";
        if (!ReadWindowText(input).empty()) return "EuroScope command input became busy before submission";
        plugin.SetASELAircraft(flightPlan);
        const auto selected = plugin.FlightPlanSelectASEL();
        if (!selected.IsValid() || callsign != selected.GetCallsign()) return "Could not select the addressed aircraft";
        SendMessageA(input, WM_SETTEXT, 0, reinterpret_cast<LPARAM>(message.c_str()));
        if (ReadWindowText(input) != message) return "Could not populate the command input";
        if (GetForegroundWindow() != mainWindow || GetFocus() != input)
            return "EuroScope lost focus before the frequency key was queued";
        auto keys = NumpadMultiplyInputs();
        const auto inserted = SendInput(static_cast<UINT>(keys.size()), keys.data(), sizeof(INPUT));
        if (inserted != keys.size()) {
            if (inserted == 1) SendInput(1, &keys[1], sizeof(INPUT));
            return "Windows rejected the numpad Multiply key input";
        }
        submission = {input, GetTickCount64()};
        return ""; // Queued only: check consumption after the UI loop runs.
    }

    bool PrivateMessageSender::SendCommandViaMessageInput(const std::string& command) {
        if (command.empty()) {
            return false;
        }

        const HWND mainWindow = FindEuroScopeMainWindow();
        if (mainWindow == nullptr) {
            Logger::Warning("PrivateMessageSender: EuroScope main window not found");
            return false;
        }

        const HWND input = GetDlgItem(mainWindow, kMessageInputControlId);
        if (input == nullptr) {
            Logger::Warning("PrivateMessageSender: message input control {} not found", kMessageInputControlId);
            return false;
        }

        const auto inputClass = ReadWindowClass(input);
        if (inputClass != "Edit") {
            Logger::Warning("PrivateMessageSender: control {} has unexpected class '{}'", kMessageInputControlId, inputClass);
            return false;
        }

        const auto existingText = ReadWindowText(input);
        if (!existingText.empty()) {
            Logger::Warning("PrivateMessageSender: message input is busy, refusing to overwrite '{}'", existingText);
            return false;
        }

        SetLastError(0);
        if (SendMessageA(input, WM_SETTEXT, 0, reinterpret_cast<LPARAM>(command.c_str())) == 0 && GetLastError() != 0) {
            Logger::Warning("PrivateMessageSender: failed to set message input text");
            return false;
        }

        SendMessageA(input, EM_SETSEL, static_cast<WPARAM>(command.size()), static_cast<LPARAM>(command.size()));

        PostReturnKey(input);
        PostReturnKey(mainWindow);

        Logger::Info("PrivateMessageSender: sent command via control {} on hwnd 0x{:08X}", kMessageInputControlId,
            static_cast<unsigned int>(reinterpret_cast<uintptr_t>(mainWindow)));
        return true;
    }
}
