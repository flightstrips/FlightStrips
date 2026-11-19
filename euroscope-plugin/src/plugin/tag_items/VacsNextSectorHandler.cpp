#include "VacsNextSectorHandler.h"

#include <algorithm>
#include <cctype>
#include <cstdio>
#include <set>
#include <sstream>

namespace FlightStrips::TagItems {
    namespace {
        constexpr int TagColorRGBDefinedValue = 1;
        constexpr COLORREF MutedTagColor = RGB(160, 160, 160);  // explicit unavailable marker, or a
                                                                  // resolved destination that cannot
                                                                  // currently be called over VACS

        // Predictions cover one entry per minute from now; near-term sector
        // changes are what matters for a calling aid, so the horizon is kept
        // short rather than walking the whole remaining flight.
        constexpr int kPredictionHorizonMinutes = 60;

        auto Normalize(std::string value) -> std::string {
            const auto first = value.find_first_not_of(" \t\r\n");
            if (first == std::string::npos) return "";
            value.erase(0, first);
            value.erase(value.find_last_not_of(" \t\r\n") + 1);
            for (auto& ch : value) ch = static_cast<char>(std::toupper(static_cast<unsigned char>(ch)));
            return value;
        }

        std::string NormalizePositionId(const std::string& value) {
            const auto id = Normalize(value);
            return std::any_of(id.begin(), id.end(), [](unsigned char ch) { return std::isalnum(ch); })
                ? id : std::string{};
        }

        template <typename ReadControllerId>
        PredictionScan ScanPrediction(int horizon, const std::string& ownPositionId,
                                      int startIndex, ReadControllerId readControllerId) {
            if (startIndex < 0) return {};
            const auto own = Normalize(ownPositionId);
            for (int i = startIndex; i < horizon; ++i) {
                const auto id = NormalizePositionId(readControllerId(i));
                if (id.empty() || id == own) continue;
                return {true, id};
            }
            return {};
        }
    }

    std::string DeriveFallbackSectorLabel(const std::string& callsign) {
        const auto normalized = Normalize(callsign);
        if (normalized.empty()) return {};
        std::vector<std::string> tokens;
        std::stringstream stream(normalized);
        std::string token;
        while (std::getline(stream, token, '_')) if (!token.empty()) tokens.push_back(token);
        if (tokens.size() >= 2) return tokens[1].substr(0, 4);
        return normalized.substr(0, 4);
    }

    int ResolvePredictionScanStart(int exitMinutes, int horizon) {
        if (exitMinutes < 0) return -1;
        if (horizon <= 0 || exitMinutes >= horizon) return -1;
        return exitMinutes;
    }

    PredictionScan ScanPredictedNextController(
        const std::vector<std::string>& controllerIdsByMinute,
        const std::string& ownPositionId,
        int startIndex) {
        return ScanPrediction(static_cast<int>(controllerIdsByMinute.size()), ownPositionId, startIndex,
                              [&](int minute) { return controllerIdsByMinute[static_cast<size_t>(minute)]; });
    }

    bool IsCoordinationStateUsable(int coordinatedNextControllerState) {
        switch (coordinatedNextControllerState) {
            case EuroScopePlugIn::COORDINATION_STATE_NONE:
            case EuroScopePlugIn::COORDINATION_STATE_REQUESTED_BY_ME:
            case EuroScopePlugIn::COORDINATION_STATE_REQUESTED_BY_OTHER:
            case EuroScopePlugIn::COORDINATION_STATE_ACCEPTED:
            case EuroScopePlugIn::COORDINATION_STATE_MANUAL_ACCEPTED:
                return true;
            default:
                // Covers COORDINATION_STATE_REFUSED and any value this
                // plugin does not recognise: never guess at an unknown state.
                return false;
        }
    }

    VacsNextSectorResolution ResolveVacsNextSector(
        bool trackedByMe,
        const std::string& ownControllerCallsign,
        const std::string& handoffPositionId, const std::string& handoffCallsign,
        const std::string& coordinatedCallsign, const std::string& coordinatedPositionId,
        bool predictionFound, const std::string& predictedPositionId, const std::string& predictedCallsign) {
        if (!trackedByMe) return {};
        const auto own = Normalize(ownControllerCallsign);
        if (!handoffCallsign.empty() && Normalize(handoffCallsign) != own) {
            const auto positionId = NormalizePositionId(handoffPositionId);
            const auto sectorId = positionId.empty() ? DeriveFallbackSectorLabel(handoffCallsign) : positionId;
            return {true, VacsNextSectorSource::Handoff, sectorId, positionId, handoffCallsign};
        }
        if (!coordinatedCallsign.empty() && Normalize(coordinatedCallsign) != own) {
            const auto positionId = NormalizePositionId(coordinatedPositionId);
            const auto sectorId = positionId.empty() ? DeriveFallbackSectorLabel(coordinatedCallsign) : positionId;
            return {true, VacsNextSectorSource::Coordinated, sectorId, positionId, coordinatedCallsign};
        }
        if (predictionFound && !predictedCallsign.empty() && Normalize(predictedCallsign) != own) {
            const auto positionId = NormalizePositionId(predictedPositionId);
            const auto sectorId = positionId.empty() ? DeriveFallbackSectorLabel(predictedCallsign) : positionId;
            return {true, VacsNextSectorSource::Prediction, sectorId, positionId, predictedCallsign};
        }
        return {};
    }

    std::optional<std::string> MatchVacsClientId(
        const std::string& fullCallsign,
        const std::string& positionId,
        const std::vector<vacs::Client>& clients) {
        const auto targetCallsign = Normalize(fullCallsign);
        const auto targetPosition = NormalizePositionId(positionId);
        if (targetCallsign.empty() && targetPosition.empty()) return std::nullopt;
        std::set<std::string> matchedIds;
        for (const auto& client : clients) {
            if (client.id.empty()) continue;
            const auto clientPosition = Normalize(client.position);
            const auto clientName = Normalize(client.name);
            const bool matchesCallsign = !targetCallsign.empty() &&
                ((!clientPosition.empty() && clientPosition == targetCallsign) ||
                 (!clientName.empty() && clientName == targetCallsign));
            const bool matchesPosition = !targetPosition.empty() &&
                ((!clientPosition.empty() && clientPosition == targetPosition) ||
                 (!clientName.empty() && clientName == targetPosition));
            if (matchesCallsign || matchesPosition) matchedIds.insert(client.id);
        }
        if (matchedIds.size() != 1) return std::nullopt;  // none, or more than one distinct client: never guess
        return *matchedIds.begin();
    }

    bool IsVacsNextSectorLabelDialable(const VacsNextSectorResolution& resolution) {
        // EuroScope's 16-byte buffer has room for "V:", 13 bytes of ID and
        // the terminator. Reserved punctuation-only markers cannot dial.
        return resolution.available && resolution.sectorId.size() <= 13 &&
               !NormalizePositionId(resolution.sectorId).empty();
    }

    std::string FormatVacsNextSectorItemString(bool trackedByMe, const VacsNextSectorResolution& resolution) {
        if (!trackedByMe) return {};
        if (!resolution.available) return "V:--";
        // Never paint a truncated prefix: it could match another complete
        // destination if the resolution changes before the controller clicks.
        if (!IsVacsNextSectorLabelDialable(resolution)) return "V:??";
        return "V:" + resolution.sectorId;
    }

    VacsCallDecision DecideVacsCallTarget(
        bool trackedByMe,
        const VacsNextSectorResolution& resolution,
        const std::optional<std::string>& matchedClientId,
        const char* paintedItemString) {
        if (!trackedByMe || !resolution.available) {
            return {VacsCallGuardResult::NoTarget, {}};
        }
        if (!IsVacsNextSectorLabelDialable(resolution)) {
            return {VacsCallGuardResult::UnusableLabel, {}};
        }
        if (!matchedClientId.has_value() || matchedClientId->empty()) {
            return {VacsCallGuardResult::NoTarget, {}};
        }
        const auto freshText = FormatVacsNextSectorItemString(trackedByMe, resolution);
        const std::string painted = paintedItemString != nullptr ? std::string(paintedItemString) : std::string{};
        if (painted.empty() || painted != freshText) {
            // Eligible labels always contain letters/digits, so a painted
            // reserved marker cannot match even if the destination changed.
            return {VacsCallGuardResult::DestinationChanged, {}};
        }
        return {VacsCallGuardResult::Dialable, *matchedClientId};
    }

    VacsNextSectorHandler::VacsNextSectorHandler(
        std::function<std::string()> ownControllerCallsign,
        std::function<std::string(const std::string&)> positionIdForCallsign,
        std::function<std::string(const std::string&)> callsignForPositionId,
        std::function<bool()> vacsReadyToCall,
        std::function<std::vector<vacs::Client>()> vacsClients)
        : ownControllerCallsign_(std::move(ownControllerCallsign)),
          positionIdForCallsign_(std::move(positionIdForCallsign)),
          callsignForPositionId_(std::move(callsignForPositionId)),
          vacsReadyToCall_(std::move(vacsReadyToCall)),
          vacsClients_(std::move(vacsClients)) {}

    VacsNextSectorResolution VacsNextSectorHandler::Resolve(EuroScopePlugIn::CFlightPlan flightPlan) const {
        if (!flightPlan.IsValid() || !flightPlan.GetTrackingControllerIsMe()) return {};

        const auto ownCallsign = ownControllerCallsign_ ? ownControllerCallsign_() : std::string{};
        const auto own = Normalize(ownCallsign);

        const std::string handoffCallsign = flightPlan.GetHandoffTargetControllerCallsign();
        const std::string handoffId = flightPlan.GetHandoffTargetControllerId();
        const bool handoffResolves = !handoffCallsign.empty() && Normalize(handoffCallsign) != own;

        std::string coordinatedCallsign;
        std::string coordinatedId;
        bool coordinationResolves = false;
        if (!handoffResolves && IsCoordinationStateUsable(flightPlan.GetCoordinatedNextControllerState())) {
            coordinatedCallsign = flightPlan.GetCoordinatedNextController();
            if (!coordinatedCallsign.empty()) {
                coordinatedId = positionIdForCallsign_ ? positionIdForCallsign_(coordinatedCallsign) : std::string{};
                coordinationResolves = Normalize(coordinatedCallsign) != own;
            }
        }

        PredictionScan scan;
        std::string predictedCallsign;
        if (!handoffResolves && !coordinationResolves) {
            // Only fetch and walk the per-minute prediction array once the
            // cheaper SDK facts above have failed to settle the result:
            // this is the expensive path, one SDK call per minute of horizon.
            const auto exitMinutes = flightPlan.GetSectorExitMinutes();
            const auto predictions = flightPlan.GetPositionPredictions();
            const auto pointsAvailable = predictions.GetPointsNumber();
            const auto horizon = std::max(0, std::min(pointsAvailable, kPredictionHorizonMinutes));
            const auto scanStart = ResolvePredictionScanStart(exitMinutes, horizon);
            if (scanStart >= 0) {
                const auto ownPositionId = std::string(flightPlan.GetTrackingControllerId());
                scan = ScanPrediction(horizon, ownPositionId, scanStart,
                                      [&](int minute) { return std::string(predictions.GetControllerId(minute)); });
                if (scan.found && callsignForPositionId_) predictedCallsign = callsignForPositionId_(scan.controllerId);
            }
        }

        return ResolveVacsNextSector(
            true, ownCallsign, handoffId, handoffCallsign, coordinatedCallsign, coordinatedId,
            scan.found, scan.controllerId, predictedCallsign);
    }

    void VacsNextSectorHandler::Handle(EuroScopePlugIn::CFlightPlan flightPlan, EuroScopePlugIn::CRadarTarget,
                                        int, int, char sItemString[16], int* pColorCode,
                                        COLORREF* pRGB, double*) {
        sItemString[0] = '\0';
        const auto trackedByMe = flightPlan.IsValid() && flightPlan.GetTrackingControllerIsMe();
        const auto resolution = Resolve(flightPlan);
        const auto text = FormatVacsNextSectorItemString(trackedByMe, resolution);
        if (text.empty()) return;  // not tracked by this controller: leave blank
        std::snprintf(sItemString, 16, "%s", text.c_str());

        if (!IsVacsNextSectorLabelDialable(resolution)) {
            // Missing destinations and labels unusable for direct calling
            // share the same muted appearance; neither can dial on click.
            if (pColorCode != nullptr) *pColorCode = TagColorRGBDefinedValue;
            if (pRGB != nullptr) *pRGB = MutedTagColor;
            return;
        }
        const auto ready = vacsReadyToCall_ && vacsReadyToCall_();
        const auto matched = ready
            ? MatchVacsClientId(resolution.fullCallsign, resolution.positionId, vacsClients_ ? vacsClients_() : std::vector<vacs::Client>{})
            : std::optional<std::string>{};
        if (matched.has_value()) return;  // callable: leave the tag's own configured colour alone
        // A destination is known but cannot currently be dialled (VACS not
        // ready, or the destination does not map to exactly one live VACS
        // client): keep the destination visible, muted rather than hidden.
        if (pColorCode != nullptr) *pColorCode = TagColorRGBDefinedValue;
        if (pRGB != nullptr) *pRGB = MutedTagColor;
    }

    VacsCallDecision VacsNextSectorHandler::ResolveCallTarget(
        EuroScopePlugIn::CFlightPlan flightPlan,
        const std::vector<vacs::Client>& clients,
        const char* paintedItemString) const {
        const auto trackedByMe = flightPlan.IsValid() && flightPlan.GetTrackingControllerIsMe();
        const auto resolution = Resolve(flightPlan);
        const auto matched = resolution.available
            ? MatchVacsClientId(resolution.fullCallsign, resolution.positionId, clients)
            : std::optional<std::string>{};
        return DecideVacsCallTarget(trackedByMe, resolution, matched, paintedItemString);
    }
}
