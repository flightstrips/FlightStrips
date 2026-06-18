#include "PrivateMessageSender.h"

#include "Logger.hpp"

#include <Windows.h>

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

    void PrivateMessageSender::SendPrivateMessage(const std::string& callsign, const std::string& message) {
        const std::string command = ".msg " + callsign + " " + message;
        if (SendCommandViaMessageInput(command)) {
            return;
        }

        Logger::Warning("PrivateMessageSender: failed to send PM via EuroScope message input");
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
