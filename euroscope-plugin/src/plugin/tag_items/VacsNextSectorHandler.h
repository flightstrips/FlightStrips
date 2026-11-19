#pragma once

#include <functional>
#include <optional>
#include <string>
#include <vector>

#include "TagItemHandler.h"
#include "vacs/VacsClient.h"

namespace FlightStrips::TagItems {
    // The EuroScope SDK has no API to read another plugin's tag item value
    // (OnGetTagItem only lets a plugin fill in its own strings), so TopSky's
    // SI cannot be read directly. This resolver derives a next-sector
    // candidate from EuroScope's handoff/coordination/prediction data.
    // Agreement with TopSky's SI, including manual overrides, needs live
    // verification. Prediction-based destinations remain estimates.
    enum class VacsNextSectorSource { None, Handoff, Coordinated, Prediction };

    struct VacsNextSectorResolution {
        bool available = false;    // true if sectorId is populated for display
        VacsNextSectorSource source = VacsNextSectorSource::None;
        std::string sectorId;      // compact display label, e.g. "KA"; may be a derived
                                    // fallback (see positionId) when EuroScope gave no short id
        std::string positionId;    // genuine EuroScope short position id, when one was given
                                    // directly by the SDK; empty if sectorId is only a derived
                                    // fallback, or if nothing is available
        std::string fullCallsign;  // resolved controller callsign; may be empty even if available
    };

    struct PredictionScan {
        bool found = false;
        std::string controllerId;
    };

    // Derives a compact, position-file-like label from a controller callsign
    // (e.g. "EKDK_KA_APP" -> "KA") for display only, used only when EuroScope's
    // own position ID for that controller cannot be resolved (not connected).
    // This never changes which controller was identified, only how its label
    // is shown, and the result is never used for VACS client matching (see
    // MatchVacsClientId), since it is a guess at a label, not a genuine
    // EuroScope position id.
    std::string DeriveFallbackSectorLabel(const std::string& callsign);

    // Returns the per-minute prediction index to start scanning from,
    // anchored at this controller's own-sector exit
    // (CFlightPlan::GetSectorExitMinutes()) rather than at "now" (minute 0).
    // Scanning from minute 0 picks up whichever sector the flight is
    // predicted to be in right now or shortly before reaching this
    // controller's own sector (e.g. an upstream centre), which is not the
    // "next" sector after this one. Returns -1 ("do not scan, prediction
    // unavailable") when the exit time is unknown (-1 from the SDK) or
    // falls beyond the fetched prediction horizon; the SDK does not document
    // whether the exit minute itself still reports the current or the next
    // controller, so ScanPredictedNextController below matches on controller
    // identity from that index onward rather than relying on the index alone.
    int ResolvePredictionScanStart(int exitMinutes, int horizon);

    // Walks position predictions (one entry per minute from now) starting at
    // startIndex, and returns the first controller different from the own
    // position. The first candidate found is used as-is: a short-lived
    // sector that only holds the flight for a minute or two before handing
    // it onward again is a legitimate next controller, not noise to filter
    // out, so nothing here second-guesses a candidate against later entries.
    PredictionScan ScanPredictedNextController(
        const std::vector<std::string>& controllerIdsByMinute,
        const std::string& ownPositionId,
        int startIndex = 0);

    // Accept a nonempty native next-controller value in known states other
    // than REFUSED. This includes NONE and pending requests as destination
    // intent; whether EuroScope/TopSky populate the field for particular
    // manual assignments in those states still needs live verification.
    bool IsCoordinationStateUsable(int coordinatedNextControllerState);

    // Pure decision combining explicit handoff/coordination facts (priority
    // order: handoff > coordination intent > predicted) with self-exclusion.
    // Every input here is already extracted from EuroScope/VACS by the
    // caller; this function touches neither.
    VacsNextSectorResolution ResolveVacsNextSector(
        bool trackedByMe,
        const std::string& ownControllerCallsign,
        const std::string& handoffPositionId, const std::string& handoffCallsign,
        const std::string& coordinatedCallsign, const std::string& coordinatedPositionId,
        bool predictionFound, const std::string& predictedPositionId, const std::string& predictedCallsign);

    // Matches a resolved controller against the live VACS client list,
    // following the same identity rules the browser uses
    // (frontend/src/vacs/match.ts): a client matches when its position or
    // its name equals either the full callsign or EuroScope's own short
    // position id. Blank client ids, and blank position/name fields, are
    // never matched against. Several rows resolving to the same client id
    // (e.g. duplicate entries) are not ambiguous; genuinely different client
    // ids are, and this never guesses between them. The plugin's own client
    // is expected to already be excluded from the given list (VacsService/
    // VacsClient already filter it out), not re-filtered here.
    std::optional<std::string> MatchVacsClientId(
        const std::string& fullCallsign,
        const std::string& positionId,
        const std::vector<vacs::Client>& clients);

    // Shared by paint and click: direct calls require an available label
    // that fits completely in EuroScope's buffer (13 bytes after "V:")
    // and is not a reserved punctuation-only marker.
    bool IsVacsNextSectorLabelDialable(const VacsNextSectorResolution& resolution);

    // Paint blank for untracked flights, V:-- for no destination, V:?? for
    // an unusable label, otherwise the complete V:<sector ID>. Reserved
    // markers can never collide with a dialable label on a later click.
    std::string FormatVacsNextSectorItemString(bool trackedByMe, const VacsNextSectorResolution& resolution);

    enum class VacsCallGuardResult { Dialable, NoTarget, DestinationChanged, UnusableLabel };

    struct VacsCallDecision {
        VacsCallGuardResult result = VacsCallGuardResult::NoTarget;
        std::string clientId;  // populated only when result == Dialable
    };

    // Pure click guard: a call is only ever dialable when the freshly
    // resolved display string is identical to what EuroScope says was
    // painted when the field was clicked (the tag function's own
    // "ItemString" parameter), and a freshly fetched VACS client list
    // resolves the target to exactly one id. A null/empty/marker
    // ItemString, or one that no longer matches what was just resolved, is
    // reported as DestinationChanged rather than NoTarget whenever a real
    // target now exists, since the caller should tell the controller to
    // refresh and retry rather than silently doing nothing. An unusable
    // current label returns UnusableLabel so the caller can suggest the
    // picker; a refresh cannot make an overlong ID fit. This guard compares
    // labels only: an identity change keeping the same label is not caught.
    VacsCallDecision DecideVacsCallTarget(
        bool trackedByMe,
        const VacsNextSectorResolution& resolution,
        const std::optional<std::string>& matchedClientId,
        const char* paintedItemString);

    // Tag item handler for the standalone "V:<sector ID>" field. Display-only;
    // the click actions in FlightStripsPlugin::OnFunctionCall re-run the same
    // Resolve() at click time through ResolveCallTarget(), so the dialled
    // target is always revalidated against the live flight plan and VACS
    // client list rather than reusing a cached value from the last paint,
    // and the dial only proceeds when that fresh result still matches what
    // was shown when the controller clicked.
    class VacsNextSectorHandler final : public TagItemHandler {
    public:
        VacsNextSectorHandler(
            std::function<std::string()> ownControllerCallsign,
            std::function<std::string(const std::string& callsign)> positionIdForCallsign,
            std::function<std::string(const std::string& positionId)> callsignForPositionId,
            std::function<bool()> vacsReadyToCall,
            std::function<std::vector<vacs::Client>()> vacsClients);

        void Handle(EuroScopePlugIn::CFlightPlan FlightPlan, EuroScopePlugIn::CRadarTarget RadarTarget,
                    int ItemCode, int TagData, char sItemString[16], int* pColorCode,
                    COLORREF* pRGB, double* pFontSize) override;

        // Re-resolves the next-sector controller for the given (freshly
        // selected) flight plan, matches it against the given (freshly
        // fetched) VACS client list, and guards it against the given
        // "ItemString" the click carried (the value EuroScope says was
        // painted). See DecideVacsCallTarget for the guard semantics.
        [[nodiscard]] VacsCallDecision ResolveCallTarget(
            EuroScopePlugIn::CFlightPlan flightPlan,
            const std::vector<vacs::Client>& clients,
            const char* paintedItemString) const;

    private:
        std::function<std::string()> ownControllerCallsign_;
        std::function<std::string(const std::string&)> positionIdForCallsign_;
        std::function<std::string(const std::string&)> callsignForPositionId_;
        std::function<bool()> vacsReadyToCall_;
        std::function<std::vector<vacs::Client>()> vacsClients_;

        [[nodiscard]] VacsNextSectorResolution Resolve(EuroScopePlugIn::CFlightPlan flightPlan) const;
    };
}
