#include "VacsPanel.h"
#include "InfoScreenObjectIds.h"

namespace FlightStrips::graphics {
    VacsLayout CalculateVacsLayout(int left, int top, bool mini, const vacs::View& view) {
        const auto rect = [=](int x1, int y1, int x2, int y2) -> RECT { return {left + x1, top + y1, left + x2, top + y2}; };
        VacsLayout result{};
        result.enabled = rect(123, 0, 155, 14);
        result.checkbox = rect(5, 18, 14, 27);
        result.preference = rect(5, 15, 155, 29);
        result.dot = mini ? rect(5, 21, 11, 27) : rect(5, 34, 11, 40);
        result.status = mini ? rect(16, 16, 73, 32) : rect(16, 30, 155, 44);
        if (view.state == vacs::State::Incoming) {
            if (mini) result.status = rect(16, 16, 54, 32);
            result.buttons.push_back({mini ? rect(58, 17, 104, 31) : rect(5, 46, 78, 62), InfoScreenObjectIds::VacsAnswer, "Answer"});
            result.buttons.push_back({mini ? rect(108, 17, 155, 31) : rect(82, 46, 155, 62), InfoScreenObjectIds::VacsReject, "Reject"});
        } else if (view.state == vacs::State::Connected || view.state == vacs::State::Connecting) {
            if (mini) result.status = rect(16, 16, 101, 32);
            result.buttons.push_back({mini ? rect(105, 17, 155, 31) : rect(5, 46, 155, 62), InfoScreenObjectIds::VacsEnd, mini ? "End" : "End call"});
        } else {
            const bool outgoing = view.state == vacs::State::Outgoing;
            result.buttons.push_back({mini ? rect(77, 17, 155, 31) : rect(5, 46, 155, 62), outgoing ? InfoScreenObjectIds::VacsCancel : InfoScreenObjectIds::VacsCall, outgoing ? "Cancel" : "Call..."});
        }
        return result;
    }
    void DrawVacsPanel(EuroScopePlugIn::CRadarScreen* screen, Graphics& graphics, const Colors& colors,
                       int left, int top, bool mini, bool enabled, bool showMini, const vacs::View& view) {
        const auto layout = CalculateVacsLayout(left, top, mini, view);
        Gdiplus::SolidBrush muted(Gdiplus::Color(135, 135, 139));
        const auto button = [&](const RECT& rect, const std::string& label, bool active) {
            graphics.FillRect(colors.headerBrush.get(), rect);
            graphics.DrawRect(colors.separatorPen.get(), rect);
            graphics.DrawString(graphics.FitStringToWidth(label, rect, true), rect, active ? colors.whiteBrush.get() : &muted, Gdiplus::StringAlignmentCenter, true);
        };
        if (!mini) {
            graphics.DrawString("VACS", {left + 5, top, left + 118, top + 14}, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
            button(layout.enabled, enabled ? "On" : "Off", true);
            if (screen) screen->AddScreenObject(InfoScreenObjectIds::VacsEnabled, "", layout.enabled, false, "Enable/disable remote controls; does not end desktop calls.");
            graphics.DrawRect(colors.buttonPen.get(), layout.checkbox);
            if (showMini) graphics.DrawXButton(colors.buttonPen.get(), layout.checkbox);
            graphics.DrawString("Show in mini", {left + 19, top + 15, left + 155, top + 29}, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
            if (screen) screen->AddScreenObject(InfoScreenObjectIds::VacsMiniPreference, "", layout.preference, false, "Saved preference; does not change a call.");
        } else {
            graphics.FillRect(colors.backgroundBrush.get(), {left, top + 15, left + 160, top + 33});
        }
        const Gdiplus::Brush* dot = colors.orangeBrush.get();
        if (view.state == vacs::State::Disabled) dot = &muted;
        else if (view.state == vacs::State::Unavailable) dot = colors.redBrush.get();
        else if (view.state == vacs::State::Idle || view.state == vacs::State::Connected) dot = colors.greenBrush.get();
        graphics.FillEllipse(dot, layout.dot);
        const auto text = mini ? view.mini : view.label;
        graphics.DrawString(graphics.FitStringToWidth(text, layout.status, true), layout.status, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear, true);
        if (screen) screen->AddScreenObject(InfoScreenObjectIds::VacsStatus, "", layout.status, false, view.tooltip.c_str());
        for (const auto& action : layout.buttons) {
            button(action.rect, view.pending ? (mini && view.state == vacs::State::Incoming ? "Wait" : "Wait...") : action.label, view.actionable);
            if (view.actionable) if (screen) screen->AddScreenObject(action.objectId, view.callId.c_str(), action.rect, false, view.tooltip.c_str());
        }
    }
}
