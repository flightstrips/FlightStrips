#include "StandTimelinePopup.h"

#include <algorithm>
#include <cctype>
#include <chrono>
#include <format>
#include <nlohmann/json.hpp>
#include <optional>
#include <string_view>

#include "InfoScreenObjectIds.h"
#include "http/Http.h"

namespace FlightStrips::graphics {
    namespace {
        constexpr int PopupWidth = 360;
        constexpr int HeaderHeight = 18;
        constexpr int StatusHeight = 17;
        constexpr int AxisHeight = 18;
        constexpr int RowHeight = 24;
        constexpr int FooterHeight = 17;
        constexpr size_t MaximumVisibleRows = 7;
        constexpr int TimelineHalfRangeMinutes = 120;
        using UtcMinutes = std::chrono::time_point<std::chrono::system_clock, std::chrono::minutes>;

        std::string UrlEncode(const std::string& value) {
            constexpr char Hex[] = "0123456789ABCDEF";
            std::string result;
            for (const unsigned char ch : value) {
                if (std::isalnum(ch) || ch == '-' || ch == '_' || ch == '.' || ch == '~') {
                    result.push_back(static_cast<char>(ch));
                } else {
                    result += std::format("%{}{}", Hex[ch >> 4], Hex[ch & 0x0F]);
                }
            }
            return result;
        }

        std::string ApiBaseUrl(const std::string& configuredUrl) {
            const auto schemeEnd = configuredUrl.find("://");
            if (schemeEnd == std::string::npos) {
                return {};
            }

            const auto pathStart = configuredUrl.find('/', schemeEnd + 3);
            auto origin = configuredUrl.substr(0, pathStart);
            if (origin.starts_with("wss://")) {
                origin.replace(0, 6, "https://");
            } else if (origin.starts_with("ws://")) {
                origin.replace(0, 5, "http://");
            } else if (!origin.starts_with("https://") && !origin.starts_with("http://")) {
                return {};
            }
            return origin;
        }

        std::string ReadString(const nlohmann::json& value, const char* field) {
            if (!value.is_object()) {
                return {};
            }
            const auto found = value.find(field);
            return found != value.end() && found->is_string() ? found->get<std::string>() : std::string{};
        }

        std::string ClockLabel(const std::string& value) {
            if (value.size() == 4 &&
                std::all_of(value.begin(), value.end(), [](const unsigned char ch) { return std::isdigit(ch) != 0; })) {
                return value.substr(0, 2) + ":" + value.substr(2, 2);
            }
            if (value.size() >= 16 && value[10] == 'T') {
                return value.substr(11, 5);
            }
            return value;
        }

        std::optional<UtcMinutes> ParseUtcMinutes(const std::string& value) {
            if (value.size() < 16 || value[4] != '-' || value[7] != '-' || value[10] != 'T' || value[13] != ':') {
                return std::nullopt;
            }
            const auto digits = std::string_view(value).substr(0, 16);
            for (size_t index = 0; index < digits.size(); ++index) {
                if (index != 4 && index != 7 && index != 10 && index != 13 && std::isdigit(static_cast<unsigned char>(digits[index])) == 0) {
                    return std::nullopt;
                }
            }
            const std::chrono::year_month_day date{
                std::chrono::year{std::stoi(value.substr(0, 4))},
                std::chrono::month{static_cast<unsigned>(std::stoi(value.substr(5, 2)))},
                std::chrono::day{static_cast<unsigned>(std::stoi(value.substr(8, 2)))}};
            const auto hour = std::stoi(value.substr(11, 2));
            const auto minute = std::stoi(value.substr(14, 2));
            if (!date.ok() || hour > 23 || minute > 59) {
                return std::nullopt;
            }
            const auto day = std::chrono::sys_days{date};
            const auto time = day + std::chrono::hours{hour} + std::chrono::minutes{minute};
            return std::chrono::time_point_cast<std::chrono::minutes>(time);
        }

        StandTimelineData ParseTimeline(const nlohmann::json& payload,
                                        const std::string& airport,
                                        const std::string& sessionName) {
            StandTimelineData result;
            result.updatedAt = ClockLabel(ReadString(payload, "generated_at"));
            const auto sessions = payload.find("sessions");
            if (sessions == payload.end() || !sessions->is_array()) {
                result.error = "Stand timeline response is incomplete.";
                return result;
            }

            const auto session = std::find_if(sessions->begin(), sessions->end(), [&](const auto& item) {
                return ReadString(item, "airport") == airport && ReadString(item, "name") == sessionName;
            });
            if (session == sessions->end()) {
                result.error = "No stand data for this session.";
                return result;
            }

            const auto now = std::chrono::floor<std::chrono::minutes>(std::chrono::system_clock::now());
            const auto rangeStart = now - std::chrono::minutes{TimelineHalfRangeMinutes};
            const auto rangeEnd = now + std::chrono::minutes{TimelineHalfRangeMinutes};
            if (const auto assignments = session->find("assignments");
                assignments != session->end() && assignments->is_array()) {
                for (const auto& assignment : *assignments) {
                    const auto stand = ReadString(assignment, "stand");
                    const auto callsign = ReadString(assignment, "callsign");
                    if (stand.empty() || callsign.empty()) {
                        continue;
                    }

                    const auto direction = ReadString(assignment, "direction");
                    const auto stage = ReadString(assignment, "stage");
                    const auto eta = ReadString(assignment, "eta");
                    const auto assignedAt = ReadString(assignment, "assigned_at");
                    const auto expiry = ReadString(assignment, "expires_at");
                    const auto plannedRelease = ReadString(assignment, "planned_release_at");
                    const auto tobt = ReadString(assignment, "departure_tobt");
                    const auto tsat = ReadString(assignment, "departure_tsat");
                    const auto createdAt = ReadString(assignment, "created_at");
                    const auto start = ParseUtcMinutes(direction == "ARRIVAL" && !eta.empty()
                        ? eta
                        : (!assignedAt.empty() ? assignedAt : createdAt));
                    const auto parsedExpiry = ParseUtcMinutes(expiry);
                    const auto parsedEta = ParseUtcMinutes(eta);
                    const auto parsedRelease = ParseUtcMinutes(plannedRelease);
                    if (!start || (parsedExpiry && *parsedExpiry < rangeStart)) {
                        continue;
                    }

                    auto end = parsedExpiry;
                    if (!end && direction == "ARRIVAL" && parsedEta) {
                        end = *parsedEta + std::chrono::minutes{30};
                    } else if (!end && direction == "DEPARTURE" && parsedRelease &&
                               *parsedRelease > now && *parsedRelease <= rangeEnd) {
                        end = parsedRelease;
                    } else if (!end) {
                        end = now + std::chrono::minutes{30};
                    }
                    if (*start > rangeEnd || *end < rangeStart || *end < *start) {
                        continue;
                    }

                    const bool active = *start <= now && *end >= now;
                    const auto startOffset = std::chrono::duration_cast<std::chrono::minutes>(*start - now).count();
                    const auto endOffset = std::chrono::duration_cast<std::chrono::minutes>(*end - now).count();
                    auto detail = direction + " / " + stage;
                    if (direction == "DEPARTURE") {
                        const auto departureTime = !tsat.empty() ? tsat : tobt;
                        if (!departureTime.empty()) {
                            detail += " / " + ClockLabel(departureTime);
                        }
                    }
                    result.rows.push_back({
                        .stand = stand,
                        .callsign = callsign,
                        .detail = detail,
                        .startOffsetMinutes = static_cast<int>(std::clamp<long long>(startOffset, -TimelineHalfRangeMinutes, TimelineHalfRangeMinutes)),
                        .endOffsetMinutes = static_cast<int>(std::clamp<long long>(endOffset, -TimelineHalfRangeMinutes, TimelineHalfRangeMinutes)),
                        .active = active,
                        .blocked = false,
                        .sortKey = direction == "ARRIVAL" && !eta.empty() ? eta : (!assignedAt.empty() ? assignedAt : createdAt),
                    });
                }
            }

            if (const auto blocks = session->find("blocks"); blocks != session->end() && blocks->is_array()) {
                for (const auto& block : *blocks) {
                    const auto stand = ReadString(block, "stand");
                    if (stand.empty()) {
                        continue;
                    }
                    const auto createdAt = ReadString(block, "created_at");
                    const auto expiry = ReadString(block, "expires_at");
                    const auto start = ParseUtcMinutes(createdAt);
                    const auto parsedExpiry = ParseUtcMinutes(expiry);
                    if (!start || (parsedExpiry && *parsedExpiry < rangeStart) || *start > rangeEnd) {
                        continue;
                    }
                    const auto blockType = ReadString(block, "block_type");
                    const auto reason = ReadString(block, "reason");
                    const auto callsign = ReadString(block, "callsign");
                    const auto end = parsedExpiry.value_or(std::min(now, rangeEnd));
                    if (end < rangeStart || end < *start) {
                        continue;
                    }
                    const bool active = *start <= now && end >= now;
                    const auto detail = reason.empty() ? blockType : blockType + " / " + reason;
                    const auto startOffset = std::chrono::duration_cast<std::chrono::minutes>(*start - now).count();
                    const auto endOffset = std::chrono::duration_cast<std::chrono::minutes>(end - now).count();
                    result.rows.push_back({
                        .stand = stand,
                        .callsign = callsign.empty() ? "-" : callsign,
                        .detail = detail,
                        .startOffsetMinutes = static_cast<int>(std::clamp<long long>(startOffset, -TimelineHalfRangeMinutes, TimelineHalfRangeMinutes)),
                        .endOffsetMinutes = static_cast<int>(std::clamp<long long>(endOffset, -TimelineHalfRangeMinutes, TimelineHalfRangeMinutes)),
                        .active = active,
                        .blocked = true,
                        .sortKey = createdAt,
                    });
                }
            }

            std::stable_sort(result.rows.begin(), result.rows.end(), [](const auto& lhs, const auto& rhs) {
                if (lhs.active != rhs.active) {
                    return lhs.active;
                }
                if (lhs.sortKey != rhs.sortKey) {
                    return lhs.sortKey < rhs.sortKey;
                }
                return lhs.stand < rhs.stand;
            });
            return result;
        }
    }

    StandTimelineData FetchStandTimeline(const std::string& baseUrl,
                                         const std::string& airport,
                                         const std::string& sessionName,
                                         const std::string& accessToken) {
        if (accessToken.empty()) {
            return {.error = "Sign in to FlightStrips to view stand activity."};
        }

        const auto origin = ApiBaseUrl(baseUrl);
        if (origin.empty()) {
            return {.error = "Stand timeline API URL is invalid."};
        }

        const auto url = std::format("{}/api/stand/status?airport={}&session_name={}",
                                     origin,
                                     UrlEncode(airport),
                                     UrlEncode(sessionName));
        const auto response = http::Http::Get(url, accessToken, 3);
        if (response.status_code < 0) {
            return {.error = std::format("Stand timeline unavailable: {}", response.content)};
        }
        if (response.status_code != 200) {
            return {.error = response.status_code == 401
                ? "Sign in to FlightStrips to view stand activity."
                : std::format("Stand timeline request failed (HTTP {}).", response.status_code)};
        }

        const auto payload = nlohmann::json::parse(response.content, nullptr, false);
        if (payload.is_discarded() || !payload.is_object()) {
            return {.error = "Stand timeline response could not be read."};
        }
        return ParseTimeline(payload, airport, sessionName);
    }

    StandTimelinePopupSize CalculateStandTimelinePopupSize(const StandTimelineData& data) {
        const auto rows = std::min(data.rows.size(), MaximumVisibleRows);
        const auto height = HeaderHeight + StatusHeight + AxisHeight + static_cast<int>(rows) * RowHeight + FooterHeight + 7;
        return {PopupWidth, std::max(90, height)};
    }

    void DrawStandTimelinePopup(EuroScopePlugIn::CRadarScreen& screen,
                                Graphics& graphics,
                                const Colors& colors,
                                const StandTimelinePopupState& state,
                                const StandTimelineData& data,
                                const bool loading) {
        const Gdiplus::SolidBrush popupBackground(Gdiplus::Color(55, 53, 55));
        const Gdiplus::SolidBrush popupHeader(Gdiplus::Color(39, 39, 39));
        const Gdiplus::SolidBrush secondaryText(Gdiplus::Color(190, 190, 190));
        const Gdiplus::SolidBrush activeBrush(Gdiplus::Color(70, 140, 95));
        const Gdiplus::SolidBrush blockBrush(Gdiplus::Color(176, 116, 44));
        const Gdiplus::SolidBrush futureBrush(Gdiplus::Color(70, 91, 115));
        const Gdiplus::Pen borderPen(Gdiplus::Color(125, 125, 125), 0.75f);
        const Gdiplus::Pen nowPen(Gdiplus::Color(210, 210, 210), 0.75f);
        const auto size = CalculateStandTimelinePopupSize(data);
        const int left = state.posX;
        const int top = state.posY;
        const RECT popupRect{left, top, left + size.width, top + size.height};
        const RECT headerRect{left, top, left + size.width, top + HeaderHeight};

        graphics.FillRect(&popupBackground, popupRect);
        graphics.DrawRect(&borderPen, popupRect);
        screen.AddScreenObject(InfoScreenObjectIds::StandTimelinePopupBackground, "", popupRect, false, nullptr);
        graphics.FillRect(&popupHeader, headerRect);
        graphics.DrawString("STAND TIMELINE", {left + 6, top, left + 185, top + HeaderHeight},
                            colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
        screen.AddScreenObject(InfoScreenObjectIds::StandTimelinePopupWindow, "", headerRect, true, nullptr);

        const RECT refreshRect{left + size.width - 34, top + 2, left + size.width - 18, top + HeaderHeight - 2};
        const RECT closeRect{left + size.width - 15, top + 2, left + size.width - 3, top + HeaderHeight - 2};
        graphics.DrawString("R", refreshRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentCenter);
        graphics.DrawXButton(colors.buttonPen.get(), closeRect);
        screen.AddScreenObject(InfoScreenObjectIds::StandTimelinePopupRefresh, "", refreshRect, false, nullptr);
        screen.AddScreenObject(InfoScreenObjectIds::StandTimelinePopupClose, "", closeRect, false, nullptr);

        const std::string status = loading
            ? "Loading stand activity..."
            : !data.error.empty()
                ? data.error
                : std::format("Updated {} UTC / {} events (showing {})",
                              data.updatedAt,
                              data.rows.size(),
                              std::min(data.rows.size(), MaximumVisibleRows));
        graphics.DrawString(graphics.FitStringToWidth(status, {left + 6, top + HeaderHeight, left + size.width - 6, top + HeaderHeight + StatusHeight}),
                            {left + 6, top + HeaderHeight, left + size.width - 6, top + HeaderHeight + StatusHeight},
                            &secondaryText,
                            Gdiplus::StringAlignmentNear);

        const int axisTop = top + HeaderHeight + StatusHeight;
        const int timelineLeft = left + 111;
        const int timelineRight = left + size.width - 9;
        const int timelineWidth = timelineRight - timelineLeft;
        const int nowX = timelineLeft + timelineWidth / 2;
        const int rowsTop = axisTop + AxisHeight;
        const int visibleRows = static_cast<int>(std::min(data.rows.size(), MaximumVisibleRows));
        const int rowsBottom = rowsTop + visibleRows * RowHeight;
        graphics.DrawHLine(colors.separatorPen.get(), timelineLeft, axisTop + 11, timelineRight);
        graphics.DrawVLine(colors.separatorPen.get(), timelineLeft, axisTop + 8, axisTop + 15);
        graphics.DrawVLine(colors.separatorPen.get(), timelineRight, axisTop + 8, axisTop + 15);
        graphics.DrawString("-2h", {timelineLeft, axisTop, timelineLeft + 34, axisTop + 10},
                            &secondaryText, Gdiplus::StringAlignmentNear);
        graphics.DrawString("NOW", {nowX - 20, axisTop, nowX + 20, axisTop + 10},
                            colors.whiteBrush.get(), Gdiplus::StringAlignmentCenter);
        graphics.DrawString("+2h", {timelineRight - 34, axisTop, timelineRight, axisTop + 10},
                            &secondaryText, Gdiplus::StringAlignmentFar);

        if (data.error.empty()) {
            const auto rowsToDraw = std::min(data.rows.size(), MaximumVisibleRows);
            for (size_t index = 0; index < rowsToDraw; ++index) {
                const auto& row = data.rows[index];
                const int rowTop = rowsTop + static_cast<int>(index) * RowHeight;
                const RECT nameRect{left + 6, rowTop + 1, timelineLeft - 5, rowTop + 12};
                const RECT detailRect{left + 6, rowTop + 12, timelineLeft - 5, rowTop + RowHeight - 1};
                graphics.DrawString(graphics.FitStringToWidth(row.stand + " " + row.callsign, nameRect),
                                    nameRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                graphics.DrawString(graphics.FitStringToWidth(row.detail, detailRect),
                                    detailRect, &secondaryText, Gdiplus::StringAlignmentNear);

                const auto offsetToX = [&](const int offset) {
                    return timelineLeft + (offset + TimelineHalfRangeMinutes) * timelineWidth /
                        (TimelineHalfRangeMinutes * 2);
                };
                const int barLeft = std::min(timelineRight - 2, offsetToX(row.startOffsetMinutes));
                const int barRight = std::clamp(std::max(barLeft + 2, offsetToX(row.endOffsetMinutes)),
                                                barLeft + 2,
                                                timelineRight);
                const RECT barRect{barLeft, rowTop + 8, barRight, rowTop + 15};
                const Gdiplus::Brush* barBrush = row.blocked ? &blockBrush : row.active ? &activeBrush : &futureBrush;
                graphics.FillRect(barBrush, barRect);
                graphics.DrawHLine(colors.separatorPen.get(), left + 5, rowTop + RowHeight - 1, timelineRight);
            }

            if (data.rows.empty()) {
                const RECT emptyRect{left + 8, rowsTop, left + size.width - 8, top + size.height - FooterHeight};
                graphics.DrawString("No current or planned stand activity.", emptyRect, &secondaryText,
                                    Gdiplus::StringAlignmentCenter);
            }
        }

        if (rowsBottom > rowsTop) {
            graphics.DrawVLine(&nowPen, nowX, axisTop + 13, rowsBottom);
        }
        const int footerTop = top + size.height - FooterHeight - 2;
        graphics.DrawHLine(colors.separatorPen.get(), left + 4, footerTop, left + size.width - 4);
        const RECT footerRect{left + 6, footerTop + 2, left + size.width - 6, top + size.height - 2};
        graphics.DrawString("Active now / planned upcoming / blocked", footerRect,
                            &secondaryText,
                            Gdiplus::StringAlignmentNear);
    }
}
