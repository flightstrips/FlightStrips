#include "CdmStateHandler.h"

#include <chrono>
#include <cstdlib>
#include <cstdio>
#include <format>

namespace FlightStrips::TagItems {
    namespace {
        constexpr int TAG_COLOR_RGB_DEFINED_VALUE = 1;
        constexpr COLORREF TAG_GREEN = RGB(0, 192, 0);
        constexpr COLORREF TAG_GREEN_NOT_ACTIVE = RGB(143, 216, 148);
        constexpr COLORREF TAG_GREY = RGB(108, 108, 108);
        constexpr COLORREF TAG_ORANGE = RGB(212, 133, 46);
        constexpr COLORREF TAG_YELLOW = RGB(212, 214, 7);
        constexpr COLORREF TAG_RED = RGB(190, 0, 0);
        constexpr COLORREF TAG_EOBT = RGB(182, 182, 182);
        constexpr COLORREF TAG_TTOT = RGB(182, 182, 182);
        constexpr COLORREF TAG_ASRT = RGB(182, 182, 182);
        constexpr COLORREF TAG_CTOT = RGB(212, 214, 7);

        auto ParseHHMM(const std::string& value) -> int;
        auto MinutesUntil(const std::string& hhmm) -> int;
        auto DiffMinutes(const std::string& left, const std::string& right) -> int;
        auto FormatMinutesFromNow(const std::string& hhmm) -> std::string;
        auto FormatTsatTobtDiff(const flightplan::CdmState& cdm) -> std::string;
        auto HasStatusToken(const std::string& status, const char* token) -> bool;
        auto IsFlightSuspended(const flightplan::CdmState& cdm) -> bool;
        auto HasLargeEobtTobtMismatch(const flightplan::CdmState& cdm) -> bool;
        auto ResolveTobtColor(const flightplan::CdmState& cdm) -> COLORREF;
        auto ResolveTsatFamilyColor(const flightplan::CdmState& cdm) -> COLORREF;
        auto StartupApproved(const flightplan::CdmState& cdm) -> bool;
        auto ReadyStartupRequested(const flightplan::CdmState& cdm) -> bool;
    }

    auto CdmStateHandler::ResolvePresentation(const flightplan::CdmState& cdm, const Field field, const std::string& fallbackEobt) -> Presentation {
        const auto startupApproved = StartupApproved(cdm);
        const auto& effectiveEobt = cdm.eobt.empty() ? fallbackEobt : cdm.eobt;

        switch (field) {
            case Field::Eobt:
                if (effectiveEobt.empty()) return {};
                if (IsFlightSuspended(cdm)) return {effectiveEobt, TAG_RED, true};
                if (HasLargeEobtTobtMismatch(cdm)) return {effectiveEobt, TAG_ORANGE, true};
                return {effectiveEobt, TAG_EOBT, true};
            case Field::Phase: {
                if (_stricmp(cdm.phase.c_str(), "I") == 0) return {"I", TAG_RED, true};
                if (!cdm.tsat.empty()) return {"C", TAG_GREEN, true};
                const auto& phaseBase = cdm.tobt.empty() ? effectiveEobt : cdm.tobt;
                if (phaseBase.empty() || MinutesUntil(phaseBase) <= 0) return {};
                return {"P", TAG_GREEN, true};
            }
            case Field::Tobt:
                if (cdm.tobt.empty()) return {"----", TAG_GREY, true};
                return {cdm.tobt, ResolveTobtColor(cdm), true};
            case Field::ReqTobt:
                if (cdm.req_tobt.empty()) return {};
                return {cdm.req_tobt, TAG_GREEN, true};
            case Field::Tsat:
                if (cdm.tsat.empty()) return {};
                return {cdm.tsat, ResolveTsatFamilyColor(cdm), true};
            case Field::TsatTobtDiff: {
                if (cdm.tsat.empty()) return {};
                return {FormatTsatTobtDiff(cdm), ResolveTsatFamilyColor(cdm), true};
            }
            case Field::Ttg: {
                if (cdm.tsat.empty()) return {};
                return {FormatMinutesFromNow(cdm.tsat), ResolveTsatFamilyColor(cdm), true};
            }
            case Field::Ttot:
                if (cdm.ttot.empty()) return {};
                if (startupApproved) return {cdm.ttot, TAG_GREY, true};
                return {cdm.ttot, TAG_TTOT, true};
            case Field::Ctot:
                if (!cdm.ctot.empty()) return {cdm.ctot, TAG_CTOT, true};
                if (!cdm.manual_ctot.empty()) return {cdm.manual_ctot, TAG_ORANGE, true};
                return {};
            case Field::FlowMessage:
                if (!cdm.ecfmp_id.empty()) return {cdm.ecfmp_id, TAG_YELLOW, true};
                if (!cdm.manual_ctot.empty()) return {"MAN ACT", TAG_YELLOW, true};
                return {};
            case Field::Status:
                if (cdm.status.empty()) return {};
                if (_stricmp(cdm.status.c_str(), "COMPLY") == 0) return {cdm.status, TAG_GREEN, true};
                if (_stricmp(cdm.status.c_str(), "AIRB") == 0 || IsFlightSuspended(cdm)) {
                    return {cdm.status, TAG_RED, true};
                }
                return {cdm.status, TAG_YELLOW, true};
            case Field::TobtConfirmedBy:
                if (cdm.tobt_confirmed_by.empty()) return {};
                return {cdm.tobt_confirmed_by, TAG_GREEN, true};
            case Field::Asrt:
                if (cdm.asrt.empty()) return {};
                if (startupApproved) return {cdm.asrt, TAG_GREY, true};
                return {cdm.asrt, TAG_ASRT, true};
            case Field::ReadyStartup:
                return {"RSTUP", ReadyStartupRequested(cdm) ? TAG_GREEN : TAG_RED, true};
            case Field::Tsac:
                if (cdm.tsac.empty()) return {};
                if (!cdm.tsat.empty() && std::abs(DiffMinutes(cdm.tsac, cdm.tsat)) > 5) {
                    return {cdm.tsac, TAG_ORANGE, true};
                }
                return {cdm.tsac, TAG_GREEN, true};
            case Field::Asat:
                if (cdm.asat.empty()) return {};
                if (MinutesUntil(cdm.asat) <= -5) return {cdm.asat, TAG_YELLOW, true};
                return {cdm.asat, TAG_GREEN, true};
        }

        return {};
    }

    void CdmStateHandler::Handle(
        EuroScopePlugIn::CFlightPlan FlightPlan,
        EuroScopePlugIn::CRadarTarget,
        int,
        int,
        char sItemString[16],
        int* pColorCode,
        COLORREF* pRGB,
        double*
    ) {
        if (m_flightPlanService == nullptr || !FlightPlan.IsValid()) return;
        const auto plan = m_flightPlanService->GetFlightPlan(std::string(FlightPlan.GetCallsign()));
        if (plan == nullptr) return;

        const auto fpData = FlightPlan.GetFlightPlanData();
        const std::string fallbackEobt = fpData.IsReceived() ? std::string(fpData.GetEstimatedDepartureTime()) : std::string{};
        const auto presentation = ResolvePresentation(plan->cdm, m_field, fallbackEobt);
        if (!presentation.hasValue) return;

        std::snprintf(sItemString, 16, "%s", presentation.value.c_str());
        if (pColorCode != nullptr) *pColorCode = TAG_COLOR_RGB_DEFINED_VALUE;
        if (pRGB != nullptr) *pRGB = presentation.color;
    }

    namespace {
        auto ParseHHMM(const std::string& value) -> int {
            if (value.size() != 4) return -1;
            for (const auto ch : value) {
                if (!std::isdigit(static_cast<unsigned char>(ch))) return -1;
            }

            const auto hour = std::stoi(value.substr(0, 2));
            const auto minute = std::stoi(value.substr(2, 2));
            if (hour < 0 || hour > 23 || minute < 0 || minute > 59) return -1;
            return (hour * 60) + minute;
        }

        auto MinutesUntil(const std::string& hhmm) -> int {
            const auto target = ParseHHMM(hhmm);
            if (target < 0) return 0;

            const auto now = std::chrono::system_clock::now();
            const auto nowTime = std::chrono::system_clock::to_time_t(now);
            std::tm utc{};
            gmtime_s(&utc, &nowTime);
            const auto current = (utc.tm_hour * 60) + utc.tm_min;

            auto diff = target - current;
            if (diff <= -720) diff += 1440;
            if (diff > 720) diff -= 1440;
            return diff;
        }

        auto DiffMinutes(const std::string& left, const std::string& right) -> int {
            const auto leftMinutes = ParseHHMM(left);
            const auto rightMinutes = ParseHHMM(right);
            if (leftMinutes < 0 || rightMinutes < 0) return 0;

            auto diff = leftMinutes - rightMinutes;
            if (diff <= -720) diff += 1440;
            if (diff > 720) diff -= 1440;
            return diff;
        }

        auto FormatMinutesFromNow(const std::string& hhmm) -> std::string {
            const auto minutesUntil = MinutesUntil(hhmm);
            if (minutesUntil == 0) return "0";

            const auto delta = -minutesUntil;
            return std::format("{:+d}", delta);
        }

        auto FormatTsatTobtDiff(const flightplan::CdmState& cdm) -> std::string {
            if (cdm.tsat.empty()) return {};
            if (cdm.tobt.empty()) return cdm.tsat;

            const auto diff = DiffMinutes(cdm.tsat, cdm.tobt);
            if (diff == 0) return cdm.tsat;
            return std::format("{}/{}", cdm.tsat, diff);
        }

        auto HasStatusToken(const std::string& status, const char* token) -> bool {
            if (status.empty() || token == nullptr || *token == '\0') return false;

            const auto tokenLength = std::strlen(token);
            size_t start = 0;
            while (start <= status.size()) {
                const auto end = status.find('/', start);
                const auto length = (end == std::string::npos ? status.size() : end) - start;
                if (length == tokenLength && _strnicmp(status.c_str() + start, token, tokenLength) == 0) {
                    return true;
                }
                if (end == std::string::npos) break;
                start = end + 1;
            }

            return false;
        }

        auto IsFlightSuspended(const flightplan::CdmState& cdm) -> bool {
            return cdm.status.find("FLS") != std::string::npos;
        }

        auto HasLargeEobtTobtMismatch(const flightplan::CdmState& cdm) -> bool {
            return !cdm.eobt.empty() && !cdm.tobt.empty() && std::abs(DiffMinutes(cdm.eobt, cdm.tobt)) > 5;
        }

        auto ResolveTobtColor(const flightplan::CdmState& cdm) -> COLORREF {
            if (StartupApproved(cdm)) return TAG_GREY;
            const auto minutesUntil = MinutesUntil(cdm.tobt);
            if (minutesUntil > 5) return TAG_GREEN_NOT_ACTIVE;
            if (minutesUntil <= -5) {
                if (!cdm.tsat.empty() && MinutesUntil(cdm.tsat) > -4) return TAG_GREEN;
                return TAG_YELLOW;
            }
            return TAG_GREEN;
        }

        auto ResolveTsatFamilyColor(const flightplan::CdmState& cdm) -> COLORREF {
            if (StartupApproved(cdm)) return TAG_GREY;
            const auto minutesUntil = MinutesUntil(cdm.tsat);
            if (minutesUntil > 5) return TAG_GREEN_NOT_ACTIVE;
            if (minutesUntil <= -4) return TAG_YELLOW;
            return TAG_GREEN;
        }

        auto StartupApproved(const flightplan::CdmState& cdm) -> bool {
            return !cdm.asat.empty() ||
                HasStatusToken(cdm.status, "STUP") ||
                HasStatusToken(cdm.status, "ST-UP") ||
                HasStatusToken(cdm.status, "PUSH") ||
                HasStatusToken(cdm.status, "TAXI") ||
                HasStatusToken(cdm.status, "DEPA");
        }

        auto ReadyStartupRequested(const flightplan::CdmState& cdm) -> bool {
            return !cdm.asrt.empty() || HasStatusToken(cdm.status, "REQASRT");
        }
    }
}
