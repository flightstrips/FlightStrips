#include "PdcPopup.h"

#include "flightplan/FlightPlanService.h"
#include "plugin/FlightStripsPlugin.h"
#include "runway/RunwayService.h"

namespace FlightStrips::graphics {
    namespace {
        constexpr int RequestedPopupWidth = 265;
        constexpr int RequestedPopupHeight = 215;
        constexpr int RequestedPopupHeaderHeight = 16;
        constexpr int RequestedSectionTopGap = 10;
        constexpr int RequestedSectionTextHeight = 7;
        constexpr int RequestedFieldGapAfterSection = 5;
        constexpr int RequestedFieldHeight = 16;
        constexpr int RequestedFieldRowGap = 2;
        constexpr int RequestedFieldWidth = 75;
        constexpr int RequestedLeftFieldX = 55;
        constexpr int RequestedRightFieldX = 185;
        constexpr int RequestedFullFieldWidth = RequestedRightFieldX + RequestedFieldWidth - RequestedLeftFieldX;
        constexpr int RequestedLabelInset = 5;
        constexpr int RequestedLabelGap = 5;
        constexpr int RequestedToFieldsGap = 38;
        constexpr int RequestedButtonsTop = 194;
        constexpr int RequestedButtonsLeft = 47;
        constexpr int RequestedSendButtonWidth = 70;
        constexpr int RequestedSendToRtGap = 20;
        constexpr int RequestedRtButtonWidth = 40;
        constexpr int RequestedRtToCancelGap = 5;
        constexpr int RequestedCancelButtonWidth = 50;
        constexpr int StandardPopupWidth = 132;
        constexpr int StandardPopupHeight = 165;
        constexpr int StandardPopupHeaderHeight = 16;
        constexpr int StandardPopupCallsignTop = 17;
        constexpr int StandardPopupCallsignHeight = 17;
        constexpr int StandardPopupFieldsTop = 38;
        constexpr int StandardPopupRowHeight = 18;
        constexpr int StandardPopupLabelWidth = 54;
        constexpr int StandardPopupFieldWidth = 75;
        constexpr int StandardPopupButtonsTop = 128;
        constexpr int StandardPopupButtonHeight = 18;

        [[nodiscard]] bool EqualsIgnoreCase(const std::string_view lhs, const std::string_view rhs) {
            return _stricmp(std::string(lhs).c_str(), std::string(rhs).c_str()) == 0;
        }

        [[nodiscard]] std::string ExtractAircraftType(const std::string& aircraftInfo) {
            const auto separator = aircraftInfo.find('/');
            if (separator == std::string::npos) {
                return aircraftInfo;
            }

            return aircraftInfo.substr(0, separator);
        }

        [[nodiscard]] std::string FormatHeading(const int heading) {
            return heading > 0 ? std::format("{:03}", heading) : "";
        }

        [[nodiscard]] std::string FormatClearedAltitude(const int clearedAltitude) {
            return clearedAltitude > 0 ? std::format("{:03}", clearedAltitude / 100) : "";
        }

        [[nodiscard]] bool HasRunwayMismatch(const std::string& runway,
                                             FlightStripsPlugin& plugin,
                                             runway::RunwayService* runwayService) {
            if (runway.empty() || runwayService == nullptr) {
                return false;
            }

            const auto airport = plugin.GetConnectionState().relevant_airport;
            for (const auto& activeRunway : runwayService->GetActiveRunways(airport.c_str())) {
                if (activeRunway.departure && _stricmp(activeRunway.name.c_str(), runway.c_str()) == 0) {
                    return false;
                }
            }

            return true;
        }

        [[nodiscard]] constexpr int RequestedFieldRowTop(const int sectionFieldsTop, const int rowIndex) {
            return sectionFieldsTop + rowIndex * (RequestedFieldHeight + RequestedFieldRowGap);
        }

        [[nodiscard]] constexpr int RequestedFieldRowBottom(const int sectionFieldsTop, const int rowIndex) {
            return RequestedFieldRowTop(sectionFieldsTop, rowIndex) + RequestedFieldHeight;
        }

        void DrawBeveledBorder(Graphics& graphics,
                               const RECT& rect,
                               const Gdiplus::Pen* lightPen,
                               const Gdiplus::Pen* darkPen) {
            graphics.DrawHLine(lightPen, rect.left, rect.top, rect.right - 1);
            graphics.DrawVLine(lightPen, rect.left, rect.top, rect.bottom - 1);
            graphics.DrawHLine(darkPen, rect.left, rect.bottom - 1, rect.right);
            graphics.DrawVLine(darkPen, rect.right, rect.top, rect.bottom);
        }

        void FillBeveledBox(Graphics& graphics,
                            const RECT& rect,
                            const Gdiplus::Brush* fillBrush,
                            const Gdiplus::Pen* lightPen,
                            const Gdiplus::Pen* darkPen) {
            graphics.FillRect(fillBrush, rect);
            DrawBeveledBorder(graphics, rect, lightPen, darkPen);
        }

        void DrawPopupHeader(EuroScopePlugIn::CRadarScreen& screen,
                             Graphics& graphics,
                             const RECT& headerRect,
                             const Gdiplus::Brush* backgroundBrush,
                             const Gdiplus::Brush* textBrush,
                             const char* title) {
            graphics.FillRect(backgroundBrush, headerRect);
            graphics.DrawString(title, headerRect, textBrush, Gdiplus::StringAlignmentCenter);
            screen.AddScreenObject(InfoScreenObjectIds::PdcPopupWindow, "", headerRect, true, nullptr);
        }

        void DrawStandardPdcPopup(EuroScopePlugIn::CRadarScreen& screen,
                                  Graphics& graphics,
                                  const Colors& colors,
                                  const PdcClearancePopupState& state,
                                  const PdcPopupData& data) {
            const Gdiplus::SolidBrush popupBackgroundBrush(Gdiplus::Color(78, 85, 89));
            const Gdiplus::SolidBrush popupTextBrush(Gdiplus::Color(200, 200, 200));
            const Gdiplus::SolidBrush runwayBrush(Gdiplus::Color(0xFF, 0x82, 0xB4));
            const Gdiplus::Pen lightBorderPen(Gdiplus::Color(155, 158, 159), 0.75f);
            const Gdiplus::Pen darkBorderPen(Gdiplus::Color(70, 70, 70), 0.75f);
            const int posX = state.posX;
            const int posY = state.posY;

            const RECT popupRect = {posX, posY, posX + StandardPopupWidth, posY + StandardPopupHeight};
            graphics.FillRect(&popupBackgroundBrush, popupRect);
            screen.AddScreenObject(InfoScreenObjectIds::PdcBackground, "", popupRect, false, nullptr);

            const RECT headerRect = {posX, posY, posX + StandardPopupWidth, posY + StandardPopupHeaderHeight};
            DrawPopupHeader(screen, graphics, headerRect, &popupBackgroundBrush, &popupTextBrush, "DEP CLEARANCE");
            graphics.DrawHLine(&darkBorderPen, posX + 1, posY + StandardPopupHeaderHeight, posX + StandardPopupWidth - 2);

            const RECT callsignRect = {posX + 1,
                                       posY + StandardPopupCallsignTop,
                                       posX + StandardPopupWidth - 1,
                                       posY + StandardPopupCallsignTop + StandardPopupCallsignHeight};
            graphics.DrawString(graphics.FitStringToWidth(data.callsign, callsignRect),
                                callsignRect,
                                &popupTextBrush,
                                Gdiplus::StringAlignmentCenter);
            const auto drawField = [&](const int rowIndex,
                                       const char* label,
                                       const std::string& value,
                                       const int objectId,
                                       const Gdiplus::Brush* valueBrush = nullptr) {
                const int rowY = posY + StandardPopupFieldsTop + rowIndex * StandardPopupRowHeight;
                const RECT labelRect = {posX + 3, rowY, posX + StandardPopupLabelWidth, rowY + StandardPopupRowHeight};
                const RECT valueRect = {posX + StandardPopupLabelWidth,
                                        rowY,
                                        posX + StandardPopupLabelWidth + StandardPopupFieldWidth,
                                        rowY + StandardPopupRowHeight};
                const RECT valueTextRect = {valueRect.left + 2, valueRect.top + 1, valueRect.right - 2, valueRect.bottom - 1};

                graphics.DrawString(label, labelRect, &popupTextBrush, Gdiplus::StringAlignmentNear);
                FillBeveledBox(graphics, valueRect, &popupBackgroundBrush, &darkBorderPen, &lightBorderPen);
                graphics.DrawString(graphics.FitStringToWidth(value, valueTextRect),
                                    valueTextRect,
                                    valueBrush == nullptr ? &popupTextBrush : valueBrush,
                                    Gdiplus::StringAlignmentCenter);
                screen.AddScreenObject(objectId, data.callsign.c_str(), valueRect, false, nullptr);
            };

            drawField(0, "RWY", data.runway, InfoScreenObjectIds::PdcFieldRunway,
                      (!data.esCleared && data.runwayMismatch) ? &runwayBrush : nullptr);
            drawField(1, "SID", data.sid, InfoScreenObjectIds::PdcFieldSid);
            drawField(2, "AHDG", FormatHeading(data.heading), InfoScreenObjectIds::PdcFieldHeading);
            drawField(3, "CFL", FormatClearedAltitude(data.clearedAltitude), InfoScreenObjectIds::PdcFieldCfl);
            drawField(4, "ASSR", data.assignedSquawk, InfoScreenObjectIds::PdcFieldSquawk);

            const RECT okRect = {posX, posY + StandardPopupButtonsTop, posX + StandardPopupWidth, posY + StandardPopupButtonsTop + StandardPopupButtonHeight};
            FillBeveledBox(graphics, okRect, &popupBackgroundBrush, &lightBorderPen, &darkBorderPen);
            graphics.DrawString(data.esCleared ? "OK" : "Ok",
                                okRect,
                                data.esCleared ? colors.greenBrush.get() : &popupTextBrush,
                                Gdiplus::StringAlignmentCenter);
            screen.AddScreenObject(InfoScreenObjectIds::PdcSendButton, data.callsign.c_str(), okRect, false, nullptr);

            const RECT cancelRect = {posX,
                                     posY + StandardPopupButtonsTop + StandardPopupButtonHeight,
                                     posX + StandardPopupWidth,
                                     posY + StandardPopupButtonsTop + StandardPopupButtonHeight * 2};
            FillBeveledBox(graphics, cancelRect, &popupBackgroundBrush, &lightBorderPen, &darkBorderPen);
            graphics.DrawString("Cancel", cancelRect, &popupTextBrush, Gdiplus::StringAlignmentCenter);
            screen.AddScreenObject(InfoScreenObjectIds::PdcCancelButton, data.callsign.c_str(), cancelRect, false, nullptr);

            DrawBeveledBorder(graphics, popupRect, &lightBorderPen, &darkBorderPen);
        }

        void DrawRequestedPdcField(EuroScopePlugIn::CRadarScreen& screen,
                                   Graphics& graphics,
                                   const Gdiplus::Brush* fillBrush,
                                   const Gdiplus::Brush* textBrush,
                                   const Gdiplus::Pen* lightPen,
                                   const Gdiplus::Pen* darkPen,
                                   const char* objectTag,
                                   const int posX,
                                   const int rowY,
                                   const int boxX,
                                    const char* label,
                                    const std::string& value,
                                    const int objectId = 0,
                                    const Gdiplus::Brush* valueBrush = nullptr,
                                    const int fieldWidth = RequestedFieldWidth) {
            const RECT boxRect = {posX + boxX, rowY, posX + boxX + fieldWidth, rowY + RequestedFieldHeight};
            const RECT labelRect = {posX + boxX - 45, rowY, posX + boxX - RequestedLabelGap, rowY + RequestedFieldHeight};
            const RECT valueRect = {boxRect.left + 4, boxRect.top + 1, boxRect.right - 2, boxRect.bottom - 1};

            graphics.DrawString(label, labelRect, textBrush, Gdiplus::StringAlignmentFar);
            FillBeveledBox(graphics, boxRect, fillBrush, lightPen, darkPen);
            graphics.DrawString(graphics.FitStringToWidth(value, valueRect),
                                valueRect,
                                valueBrush == nullptr ? textBrush : valueBrush,
                                Gdiplus::StringAlignmentCenter);

            if (objectId != 0) {
                screen.AddScreenObject(objectId, objectTag, boxRect, false, nullptr);
            }
        }

        void DrawRequestedPdcPopup(EuroScopePlugIn::CRadarScreen& screen,
                                   Graphics& graphics,
                                   const Colors& colors,
                                   const PdcClearancePopupState& state,
                                   const PdcPopupData& data) {
            const int posX = state.posX;
            const int posY = state.posY;
            const bool hasRequestRemarks = HasRequestRemarks(data.requestRemarks);
            const int requestRemarksOffset = hasRequestRemarks ? RequestedFieldHeight + RequestedFieldRowGap : 0;
            const int requestedButtonsTop = RequestedButtonsTop + requestRemarksOffset;
            const int requestedPopupHeight = RequestedPopupHeight + requestRemarksOffset;
            const Gdiplus::SolidBrush popupHeaderBrush(Gdiplus::Color(88, 95, 99));
            const Gdiplus::SolidBrush popupBackgroundBrush(Gdiplus::Color(78, 85, 89));
            const Gdiplus::SolidBrush popupTextBrush(Gdiplus::Color(200, 200, 200));
            const Gdiplus::SolidBrush blackBrush(Gdiplus::Color(0, 0, 0));
            const Gdiplus::SolidBrush runwayBrush(Gdiplus::Color(0xFF, 0x82, 0xB4));
            const Gdiplus::Pen lightBorderPen(Gdiplus::Color(155, 158, 159), 0.75f);
            const Gdiplus::Pen darkBorderPen(Gdiplus::Color(70, 70, 70), 0.75f);

            const RECT popupRect = {posX, posY, posX + RequestedPopupWidth, posY + requestedPopupHeight};
            graphics.FillRect(&popupBackgroundBrush, popupRect);
            graphics.DrawRect(colors.backgroundPen.get(), popupRect);
            screen.AddScreenObject(InfoScreenObjectIds::PdcBackground, "", popupRect, false, nullptr);

            const RECT headerRect = {posX, posY, posX + RequestedPopupWidth, posY + RequestedPopupHeaderHeight};
            DrawPopupHeader(screen, graphics, headerRect, &popupHeaderBrush, &blackBrush, "Departure Clearance");

            const int fromSectionTop = posY + RequestedPopupHeaderHeight + RequestedSectionTopGap;
            const int fromFieldsTop = fromSectionTop + RequestedSectionTextHeight + RequestedFieldGapAfterSection;
            const int toFieldsTop = RequestedFieldRowBottom(fromFieldsTop, hasRequestRemarks ? 2 : 1) + RequestedToFieldsGap;
            const int toSectionTop = toFieldsTop - RequestedFieldGapAfterSection - RequestedSectionTextHeight;

            const RECT fromLabelRect = {posX + RequestedLabelInset, fromSectionTop, posX + 90, fromSectionTop + RequestedSectionTextHeight};
            const RECT callsignRect = {posX + RequestedLeftFieldX, fromSectionTop, posX + RequestedPopupWidth - RequestedLabelInset, fromSectionTop + RequestedSectionTextHeight};
            graphics.DrawString("From a/c", fromLabelRect, &popupTextBrush, Gdiplus::StringAlignmentNear);
            graphics.DrawString(graphics.FitStringToWidth(data.callsign, callsignRect),
                                callsignRect,
                                &popupTextBrush,
                                Gdiplus::StringAlignmentCenter);

            DrawRequestedPdcField(screen,
                                  graphics,
                                  &popupBackgroundBrush,
                                  &popupTextBrush,
                                  &lightBorderPen,
                                  &darkBorderPen,
                                  data.callsign.c_str(),
                                  posX,
                                  RequestedFieldRowTop(fromFieldsTop, 0),
                                  RequestedLeftFieldX,
                                  "Gate",
                                  data.gate);
            DrawRequestedPdcField(screen,
                                  graphics,
                                  &popupBackgroundBrush,
                                  &popupTextBrush,
                                  &lightBorderPen,
                                  &darkBorderPen,
                                  data.callsign.c_str(),
                                  posX,
                                  RequestedFieldRowTop(fromFieldsTop, 0),
                                  RequestedRightFieldX,
                                  "ATYP",
                                  data.aircraftType);
            DrawRequestedPdcField(screen,
                                  graphics,
                                  &popupBackgroundBrush,
                                  &popupTextBrush,
                                  &lightBorderPen,
                                  &darkBorderPen,
                                  data.callsign.c_str(),
                                  posX,
                                  RequestedFieldRowTop(fromFieldsTop, 1),
                                  RequestedLeftFieldX,
                                  "ADES",
                                  data.destination);
            DrawRequestedPdcField(screen,
                                  graphics,
                                  &popupBackgroundBrush,
                                  &popupTextBrush,
                                  &lightBorderPen,
                                  &darkBorderPen,
                                  data.callsign.c_str(),
                                  posX,
                                  RequestedFieldRowTop(fromFieldsTop, 1),
                                  RequestedRightFieldX,
                                  "ATIS",
                                  data.atis);
            if (hasRequestRemarks) {
                DrawRequestedPdcField(screen,
                                      graphics,
                                      &popupBackgroundBrush,
                                      &popupTextBrush,
                                      &lightBorderPen,
                                      &darkBorderPen,
                                      data.callsign.c_str(),
                                      posX,
                                      RequestedFieldRowTop(fromFieldsTop, 2),
                                      RequestedLeftFieldX,
                                      "RMK",
                                      data.requestRemarks,
                                      0,
                                      nullptr,
                                      RequestedFullFieldWidth);
            }

            const RECT toLabelRect = {posX + RequestedLabelInset, toSectionTop, posX + 90, toSectionTop + RequestedSectionTextHeight};
            graphics.DrawString("To a/c", toLabelRect, &popupTextBrush, Gdiplus::StringAlignmentNear);

            DrawRequestedPdcField(screen,
                                  graphics,
                                  &popupBackgroundBrush,
                                  &popupTextBrush,
                                  &lightBorderPen,
                                  &darkBorderPen,
                                  data.callsign.c_str(),
                                  posX,
                                  RequestedFieldRowTop(toFieldsTop, 0),
                                  RequestedLeftFieldX,
                                  "RWY",
                                  data.runway,
                                  InfoScreenObjectIds::PdcFieldRunway,
                                  (!data.esCleared && data.runwayMismatch) ? &runwayBrush : nullptr);
            DrawRequestedPdcField(screen,
                                  graphics,
                                  &popupBackgroundBrush,
                                  &popupTextBrush,
                                  &lightBorderPen,
                                  &darkBorderPen,
                                  data.callsign.c_str(),
                                  posX,
                                  RequestedFieldRowTop(toFieldsTop, 0),
                                  RequestedRightFieldX,
                                  "SID",
                                  data.sid,
                                  InfoScreenObjectIds::PdcFieldSid);
            DrawRequestedPdcField(screen,
                                  graphics,
                                  &popupBackgroundBrush,
                                  &popupTextBrush,
                                  &lightBorderPen,
                                  &darkBorderPen,
                                  data.callsign.c_str(),
                                  posX,
                                  RequestedFieldRowTop(toFieldsTop, 1),
                                  RequestedLeftFieldX,
                                  "AHDG",
                                  FormatHeading(data.heading),
                                  InfoScreenObjectIds::PdcFieldHeading);
            DrawRequestedPdcField(screen,
                                  graphics,
                                  &popupBackgroundBrush,
                                  &popupTextBrush,
                                  &lightBorderPen,
                                  &darkBorderPen,
                                  data.callsign.c_str(),
                                  posX,
                                  RequestedFieldRowTop(toFieldsTop, 2),
                                  RequestedLeftFieldX,
                                  "CFL",
                                  FormatClearedAltitude(data.clearedAltitude),
                                  InfoScreenObjectIds::PdcFieldCfl);
            DrawRequestedPdcField(screen,
                                  graphics,
                                  &popupBackgroundBrush,
                                  &popupTextBrush,
                                  &lightBorderPen,
                                  &darkBorderPen,
                                  data.callsign.c_str(),
                                  posX,
                                  RequestedFieldRowTop(toFieldsTop, 2),
                                  RequestedRightFieldX,
                                  "ASSR",
                                  data.assignedSquawk,
                                  InfoScreenObjectIds::PdcFieldSquawk);
            DrawRequestedPdcField(screen,
                                  graphics,
                                  &popupBackgroundBrush,
                                  &popupTextBrush,
                                  &lightBorderPen,
                                  &darkBorderPen,
                                  data.callsign.c_str(),
                                  posX,
                                  RequestedFieldRowTop(toFieldsTop, 3),
                                  RequestedLeftFieldX,
                                  "RMK",
                                  data.clearanceRemarks,
                                  InfoScreenObjectIds::PdcFieldRemarks,
                                  nullptr,
                                  RequestedFullFieldWidth);

            const auto drawButton = [&](const int buttonX, const int width, const char* text, const int objectId) {
                const RECT buttonRect = {posX + buttonX, posY + requestedButtonsTop, posX + buttonX + width, posY + requestedButtonsTop + RequestedFieldHeight};
                FillBeveledBox(graphics, buttonRect, &popupBackgroundBrush, &lightBorderPen, &darkBorderPen);
                graphics.DrawString(text, buttonRect, &popupTextBrush, Gdiplus::StringAlignmentCenter);
                screen.AddScreenObject(objectId, data.callsign.c_str(), buttonRect, false, nullptr);
            };

            drawButton(RequestedButtonsLeft, RequestedSendButtonWidth, "Send MSG", InfoScreenObjectIds::PdcSendButton);
            drawButton(RequestedButtonsLeft + RequestedSendButtonWidth + RequestedSendToRtGap,
                       RequestedRtButtonWidth,
                       "R/T",
                       InfoScreenObjectIds::PdcRtButton);
            drawButton(RequestedButtonsLeft + RequestedSendButtonWidth + RequestedSendToRtGap + RequestedRtButtonWidth + RequestedRtToCancelGap,
                       RequestedCancelButtonWidth,
                       "Cancel",
                       InfoScreenObjectIds::PdcCancelButton);
        }
    }

    bool HasRequestRemarks(const std::string_view remarks) {
        return remarks.find_first_not_of(" \t\r\n") != std::string_view::npos;
    }

    bool IsRequestedPdcState(const std::string_view state) {
        return EqualsIgnoreCase(state, "REQUESTED") || EqualsIgnoreCase(state, "REQUESTED_WITH_FAULTS");
    }

    auto ResolvePdcPopupPrimaryAction(const std::string_view state, const bool alreadyClear) -> PdcPopupPrimaryAction {
        if (alreadyClear) {
            return PdcPopupPrimaryAction::None;
        }

        return IsRequestedPdcState(state)
                   ? PdcPopupPrimaryAction::IssueRequestedClearance
                   : PdcPopupPrimaryAction::SetEuroscopeClearance;
    }

    bool ShouldSendPdcRevertToVoice(const std::string_view state) {
        return IsRequestedPdcState(state);
    }

    std::optional<PdcPopupData> BuildPdcPopupData(const PdcClearancePopupState& state,
                                                  FlightStripsPlugin& plugin,
                                                  flightplan::FlightPlanService* flightPlanService,
                                                  runway::RunwayService* runwayService) {
        const auto fp = plugin.FlightPlanSelect(state.callsign.c_str());
        if (!fp.IsValid()) {
            return std::nullopt;
        }

        const auto fpData = fp.GetFlightPlanData();
        const auto cad = fp.GetControllerAssignedData();
        const auto tracked = flightPlanService == nullptr ? nullptr : flightPlanService->GetFlightPlan(state.callsign);

        PdcPopupData data{};
        data.callsign = state.callsign;
        data.gate = tracked != nullptr && !tracked->stand.empty()
                        ? tracked->stand
                        : std::string(cad.GetFlightStripAnnotation(6));
        data.destination = std::string(fpData.GetDestination());
        data.aircraftType = ExtractAircraftType(std::string(fpData.GetAircraftInfo()));
        data.atis = "";
        data.requestRemarks = tracked != nullptr ? tracked->pdc_request_remarks : "";
        data.clearanceRemarks = state.clearanceRemarks;
        data.pdcState = tracked != nullptr ? tracked->pdc_state : "";
        data.runway = std::string(fpData.GetDepartureRwy());
        data.sid = std::string(fpData.GetSidName());
        data.heading = cad.GetAssignedHeading();
        data.clearedAltitude = cad.GetClearedAltitude();
        data.assignedSquawk = std::string(cad.GetSquawk());
        data.esCleared = fp.GetClearenceFlag();
        data.runwayMismatch = HasRunwayMismatch(data.runway, plugin, runwayService);
        return data;
    }

    void DrawPdcPopup(EuroScopePlugIn::CRadarScreen& screen,
                      Graphics& graphics,
                      const Colors& colors,
                      const PdcClearancePopupState& state,
                      const PdcPopupData& data) {
        if (IsRequestedPdcState(data.pdcState)) {
            DrawRequestedPdcPopup(screen, graphics, colors, state, data);
            return;
        }

        DrawStandardPdcPopup(screen, graphics, colors, state, data);
    }
}
