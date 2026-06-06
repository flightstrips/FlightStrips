#include "PrivateMessageSender.h"

#include <Windows.h>
#include <vector>

namespace FlightStrips::messages {
    void PrivateMessageSender::SendPrivateMessage(const std::string& callsign, const std::string& message) {
        std::string command = ".msg " + callsign + " " + message;
        TypeString(command);
        PressEnter();
    }

    void PrivateMessageSender::TypeString(const std::string& text) {
        if (text.empty()) return;

        std::vector<INPUT> inputs;
        inputs.reserve(text.length() * 2);

        for (const char ch : text) {
            SHORT vkResult = VkKeyScanA(ch);
            if (vkResult == -1) continue;

            WORD vk = vkResult & 0xFF;
            bool shiftRequired = (vkResult & 0x100) != 0;

            if (shiftRequired) {
                INPUT shiftDown = {};
                shiftDown.type = INPUT_KEYBOARD;
                shiftDown.ki.wVk = VK_SHIFT;
                shiftDown.ki.dwFlags = 0;
                inputs.push_back(shiftDown);
            }

            INPUT charDown = {};
            charDown.type = INPUT_KEYBOARD;
            charDown.ki.wVk = vk;
            charDown.ki.dwFlags = 0;
            inputs.push_back(charDown);

            INPUT charUp = {};
            charUp.type = INPUT_KEYBOARD;
            charUp.ki.wVk = vk;
            charUp.ki.dwFlags = KEYEVENTF_KEYUP;
            inputs.push_back(charUp);

            if (shiftRequired) {
                INPUT shiftUp = {};
                shiftUp.type = INPUT_KEYBOARD;
                shiftUp.ki.wVk = VK_SHIFT;
                shiftUp.ki.dwFlags = KEYEVENTF_KEYUP;
                inputs.push_back(shiftUp);
            }
        }

        if (!inputs.empty()) {
            SendInput(static_cast<UINT>(inputs.size()), inputs.data(), sizeof(INPUT));
        }
    }

    void PrivateMessageSender::PressEnter() {
        PressKey(VK_RETURN, true);
        PressKey(VK_RETURN, false);
    }

    void PrivateMessageSender::PressKey(unsigned short vk, bool keyDown) {
        INPUT input = {};
        input.type = INPUT_KEYBOARD;
        input.ki.wVk = vk;
        input.ki.dwFlags = keyDown ? 0 : KEYEVENTF_KEYUP;
        SendInput(1, &input, sizeof(INPUT));
    }
}