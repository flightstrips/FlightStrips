#include "tag_items/VacsNextSectorHandler.h"

#include <gtest/gtest.h>

using FlightStrips::TagItems::DecideVacsCallTarget;
using FlightStrips::TagItems::DeriveFallbackSectorLabel;
using FlightStrips::TagItems::FormatVacsNextSectorItemString;
using FlightStrips::TagItems::IsCoordinationStateUsable;
using FlightStrips::TagItems::IsVacsNextSectorLabelDialable;
using FlightStrips::TagItems::MatchVacsClientId;
using FlightStrips::TagItems::PredictionScan;
using FlightStrips::TagItems::ResolvePredictionScanStart;
using FlightStrips::TagItems::ResolveVacsNextSector;
using FlightStrips::TagItems::ScanPredictedNextController;
using FlightStrips::TagItems::VacsCallGuardResult;
using FlightStrips::TagItems::VacsNextSectorResolution;
using FlightStrips::TagItems::VacsNextSectorSource;
using FlightStrips::vacs::Client;

TEST(VacsNextSectorHandlerTest, DerivesFallbackLabelFromConventionalCallsigns) {
    EXPECT_EQ(DeriveFallbackSectorLabel("EKDK_KA_APP"), "KA");
    EXPECT_EQ(DeriveFallbackSectorLabel("EKDK_UC_CTR"), "UC");
    EXPECT_EQ(DeriveFallbackSectorLabel("ekch_app"), "APP");
    EXPECT_EQ(DeriveFallbackSectorLabel("CTR"), "CTR");
    EXPECT_EQ(DeriveFallbackSectorLabel(""), "");
}

// ---------------------------------------------------------------------------
// ResolvePredictionScanStart (H1: anchor scanning at own-sector exit)
// ---------------------------------------------------------------------------

TEST(VacsNextSectorHandlerTest, ScanStartIsUnavailableWhenExitMinutesIsMinusOne) {
    EXPECT_EQ(ResolvePredictionScanStart(-1, 60), -1);
}

TEST(VacsNextSectorHandlerTest, ScanStartIsUnavailableWhenExitIsBeyondTheFetchedHorizon) {
    EXPECT_EQ(ResolvePredictionScanStart(60, 60), -1);
    EXPECT_EQ(ResolvePredictionScanStart(61, 60), -1);
}

TEST(VacsNextSectorHandlerTest, ScanStartIsUnavailableWhenThereIsNoHorizon) {
    EXPECT_EQ(ResolvePredictionScanStart(0, 0), -1);
}

TEST(VacsNextSectorHandlerTest, ScanStartIsTheExitIndexWhenWithinBounds) {
    EXPECT_EQ(ResolvePredictionScanStart(0, 5), 0);
    EXPECT_EQ(ResolvePredictionScanStart(4, 5), 4);
}

// ---------------------------------------------------------------------------
// ScanPredictedNextController
// ---------------------------------------------------------------------------

TEST(VacsNextSectorHandlerTest, ScanFindsNoCandidateWhenAllEntriesMatchOwnPosition) {
    const auto scan = ScanPredictedNextController({"KA", "KA", "KA"}, "KA");
    EXPECT_FALSE(scan.found);
}

TEST(VacsNextSectorHandlerTest, ScanFindsStableCandidateAfterOwnPosition) {
    const auto scan = ScanPredictedNextController({"KA", "KA", "UC", "UC", "UC"}, "KA");
    EXPECT_TRUE(scan.found);
    EXPECT_EQ(scan.controllerId, "UC");
}

TEST(VacsNextSectorHandlerTest, ScanIsCaseInsensitiveAndTrims) {
    const auto scan = ScanPredictedNextController({" ka ", "uc", "UC"}, "  KA  ");
    EXPECT_TRUE(scan.found);
    EXPECT_EQ(scan.controllerId, "UC");
}

TEST(VacsNextSectorHandlerTest, ScanToleratesGapsInPredictionData) {
    const auto scan = ScanPredictedNextController({"KA", "UC", "", "UC"}, "KA");
    EXPECT_TRUE(scan.found);
    EXPECT_EQ(scan.controllerId, "UC");
}

TEST(VacsNextSectorHandlerTest, ScanReturnsNotFoundForEmptyPredictions) {
    EXPECT_FALSE(ScanPredictedNextController({}, "KA").found);
}

// H1 regression: minute 0 shows an upstream centre the flight passes through
// before reaching this controller's own sector (own == "KA"); scanning from
// minute 0 would wrongly pick that upstream sector as "next".
TEST(VacsNextSectorHandlerTest, ScanFromMinuteZeroWronglyPicksUpstreamSectorBeforeOwn) {
    const std::vector<std::string> predictions = {"CTR", "CTR", "KA", "KA", "UC"};
    const auto scanFromZero = ScanPredictedNextController(predictions, "KA", /*startIndex=*/0);
    EXPECT_TRUE(scanFromZero.found);
    EXPECT_EQ(scanFromZero.controllerId, "CTR");  // the bug: calls upstream, not the real next sector
}

TEST(VacsNextSectorHandlerTest, ScanAnchoredAtOwnExitSkipsUpstreamSectorAndFindsRealNext) {
    const std::vector<std::string> predictions = {"CTR", "CTR", "KA", "KA", "UC"};
    const auto exitMinutes = ResolvePredictionScanStart(/*exitMinutes=*/4, /*horizon=*/static_cast<int>(predictions.size()));
    const auto scan = ScanPredictedNextController(predictions, "KA", exitMinutes);
    EXPECT_TRUE(scan.found);
    EXPECT_EQ(scan.controllerId, "UC");
}

// M2 regression: a short-lived sector must not be rejected as "flickering".
TEST(VacsNextSectorHandlerTest, ScanAcceptsASmallSectorHeldForOnlyOneMinute) {
    const auto scan = ScanPredictedNextController({"KA", "UC", "N", "N"}, "KA", /*startIndex=*/1);
    EXPECT_TRUE(scan.found);
    EXPECT_EQ(scan.controllerId, "UC");
}

TEST(VacsNextSectorHandlerTest, ScanAcceptsReEntryIntoTheOwnSectorAfterTheFirstCandidate) {
    const auto scan = ScanPredictedNextController({"KA", "UC", "KA"}, "KA", /*startIndex=*/1);
    EXPECT_TRUE(scan.found);
    EXPECT_EQ(scan.controllerId, "UC");
}

// ---------------------------------------------------------------------------
// IsCoordinationStateUsable (M1: broaden beyond ACCEPTED/MANUAL_ACCEPTED)
// ---------------------------------------------------------------------------

TEST(VacsNextSectorHandlerTest, CoordinationStateUsableForEveryNonRefusedKnownState) {
    EXPECT_TRUE(IsCoordinationStateUsable(EuroScopePlugIn::COORDINATION_STATE_NONE));
    EXPECT_TRUE(IsCoordinationStateUsable(EuroScopePlugIn::COORDINATION_STATE_REQUESTED_BY_ME));
    EXPECT_TRUE(IsCoordinationStateUsable(EuroScopePlugIn::COORDINATION_STATE_REQUESTED_BY_OTHER));
    EXPECT_TRUE(IsCoordinationStateUsable(EuroScopePlugIn::COORDINATION_STATE_ACCEPTED));
    EXPECT_TRUE(IsCoordinationStateUsable(EuroScopePlugIn::COORDINATION_STATE_MANUAL_ACCEPTED));
}

TEST(VacsNextSectorHandlerTest, CoordinationStateRejectedForRefusedOrUnknown) {
    EXPECT_FALSE(IsCoordinationStateUsable(EuroScopePlugIn::COORDINATION_STATE_REFUSED));
    EXPECT_FALSE(IsCoordinationStateUsable(0));
    EXPECT_FALSE(IsCoordinationStateUsable(999));
}

// ---------------------------------------------------------------------------
// ResolveVacsNextSector
// ---------------------------------------------------------------------------

TEST(VacsNextSectorHandlerTest, ResolveReturnsUnavailableWhenNotLocallyTracked) {
    const auto resolution = ResolveVacsNextSector(
        false, "EKDK_KA_APP", "UC", "EKDK_UC_CTR", "", "", false, "", "");
    EXPECT_FALSE(resolution.available);
    EXPECT_EQ(resolution.source, VacsNextSectorSource::None);
}

TEST(VacsNextSectorHandlerTest, ResolvePrefersHandoffOverCoordinationAndPrediction) {
    const auto resolution = ResolveVacsNextSector(
        true, "EKDK_KA_APP",
        "UC", "EKDK_UC_CTR",
        "EKDK_S_CTR", "S",
        true, "N", "EKDK_N_APP");
    EXPECT_TRUE(resolution.available);
    EXPECT_EQ(resolution.source, VacsNextSectorSource::Handoff);
    EXPECT_EQ(resolution.sectorId, "UC");
    EXPECT_EQ(resolution.positionId, "UC");
    EXPECT_EQ(resolution.fullCallsign, "EKDK_UC_CTR");
}

TEST(VacsNextSectorHandlerTest, ResolveFallsBackToDerivedHandoffLabelWhenShortIdMissing) {
    const auto resolution = ResolveVacsNextSector(
        true, "EKDK_KA_APP",
        "", "EKDK_UC_CTR",
        "", "",
        false, "", "");
    EXPECT_TRUE(resolution.available);
    EXPECT_EQ(resolution.source, VacsNextSectorSource::Handoff);
    EXPECT_EQ(resolution.sectorId, "UC");       // derived display fallback
    EXPECT_EQ(resolution.positionId, "");       // but not a genuine position id for matching
    EXPECT_EQ(resolution.fullCallsign, "EKDK_UC_CTR");
}

TEST(VacsNextSectorHandlerTest, ResolveSkipsHandoffTargetingSelf) {
    const auto resolution = ResolveVacsNextSector(
        true, "EKDK_KA_APP",
        "KA", "EKDK_KA_APP",
        "EKDK_S_CTR", "S",
        false, "", "");
    EXPECT_TRUE(resolution.available);
    EXPECT_EQ(resolution.source, VacsNextSectorSource::Coordinated);
    EXPECT_EQ(resolution.sectorId, "S");
}

TEST(VacsNextSectorHandlerTest, ResolveFallsBackToDerivedLabelWhenCoordinatedIdUnresolved) {
    const auto resolution = ResolveVacsNextSector(
        true, "EKDK_KA_APP",
        "", "",
        "EKDK_UC_CTR", "",
        false, "", "");
    EXPECT_TRUE(resolution.available);
    EXPECT_EQ(resolution.source, VacsNextSectorSource::Coordinated);
    EXPECT_EQ(resolution.sectorId, "UC");
    EXPECT_EQ(resolution.positionId, "");
}

TEST(VacsNextSectorHandlerTest, ResolveSkipsCoordinationTargetingSelfAndFallsBackToPrediction) {
    const auto resolution = ResolveVacsNextSector(
        true, "EKDK_KA_APP",
        "", "",
        "EKDK_KA_APP", "KA",
        true, "UC", "EKDK_UC_CTR");
    EXPECT_TRUE(resolution.available);
    EXPECT_EQ(resolution.source, VacsNextSectorSource::Prediction);
    EXPECT_EQ(resolution.sectorId, "UC");
}

TEST(VacsNextSectorHandlerTest, ResolveFallsBackToPredictionWhenNothingElseIsExplicit) {
    const auto resolution = ResolveVacsNextSector(
        true, "EKDK_KA_APP",
        "", "", "", "",
        true, "UC", "EKDK_UC_CTR");
    EXPECT_TRUE(resolution.available);
    EXPECT_EQ(resolution.source, VacsNextSectorSource::Prediction);
    EXPECT_EQ(resolution.sectorId, "UC");
    EXPECT_EQ(resolution.positionId, "UC");
}

TEST(VacsNextSectorHandlerTest, ResolveIsExplicitlyUnavailableWhenNothingIsFound) {
    const auto resolution = ResolveVacsNextSector(
        true, "EKDK_KA_APP", "", "", "", "", false, "", "");
    EXPECT_FALSE(resolution.available);
    EXPECT_EQ(resolution.source, VacsNextSectorSource::None);
}

TEST(VacsNextSectorHandlerTest, ResolveIgnoresAPredictionThatNeverResolvedToACallsign) {
    // The predicted controller id could not be resolved to a live callsign
    // (e.g. not connected): never guess a destination from a bare position id.
    const auto resolution = ResolveVacsNextSector(
        true, "EKDK_KA_APP",
        "", "", "", "",
        true, "UC", "");
    EXPECT_FALSE(resolution.available);
}

// ---------------------------------------------------------------------------
// MatchVacsClientId (M3: match client.position or client.name against
// either the full callsign or EuroScope's short position id)
// ---------------------------------------------------------------------------

TEST(VacsNextSectorHandlerTest, MatchFindsUniqueCaseInsensitiveClientByFullCallsignPosition) {
    const std::vector<Client> clients = {
        {"client-1", "Approach", "119.800", "EKDK_UC_CTR"},
        {"client-2", "Centre", "121.500", "EKDK_S_CTR"},
    };
    EXPECT_EQ(MatchVacsClientId("ekdk_uc_ctr", "", clients), "client-1");
    EXPECT_EQ(MatchVacsClientId("EKDK_S_CTR", "", clients), "client-2");
}

TEST(VacsNextSectorHandlerTest, MatchFindsClientByNameAgainstFullCallsign) {
    const std::vector<Client> clients = {{"client-1", "EKDK_UC_CTR", "119.800", ""}};
    EXPECT_EQ(MatchVacsClientId("EKDK_UC_CTR", "", clients), "client-1");
}

TEST(VacsNextSectorHandlerTest, MatchFindsClientByPositionAgainstShortPositionId) {
    const std::vector<Client> clients = {{"client-1", "Approach", "119.800", "UC"}};
    EXPECT_EQ(MatchVacsClientId("", "uc", clients), "client-1");
}

TEST(VacsNextSectorHandlerTest, MatchFindsClientByNameAgainstShortPositionId) {
    const std::vector<Client> clients = {{"client-1", "UC", "119.800", ""}};
    EXPECT_EQ(MatchVacsClientId("", "uc", clients), "client-1");
}

TEST(VacsNextSectorHandlerTest, MatchReturnsNulloptWhenNoClientMatches) {
    const std::vector<Client> clients = {{"client-1", "Approach", "119.800", "EKDK_UC_CTR"}};
    EXPECT_FALSE(MatchVacsClientId("EKDK_S_CTR", "S", clients).has_value());
}

TEST(VacsNextSectorHandlerTest, MatchNeverGuessesBetweenTwoDifferentClientsOnTheSamePosition) {
    const std::vector<Client> clients = {
        {"client-1", "Approach A", "119.800", "EKDK_UC_CTR"},
        {"client-2", "Approach B", "119.800", "EKDK_UC_CTR"},
    };
    EXPECT_FALSE(MatchVacsClientId("EKDK_UC_CTR", "", clients).has_value());
}

TEST(VacsNextSectorHandlerTest, MatchTreatsTheSameClientIdMatchingTwiceAsUnambiguous) {
    // Same client, matching via both callsign and position keys (duplicate
    // row or multiple matching fields): one distinct id, not ambiguous.
    const std::vector<Client> clients = {{"client-1", "EKDK_UC_CTR", "119.800", "UC"}};
    EXPECT_EQ(MatchVacsClientId("EKDK_UC_CTR", "UC", clients), "client-1");
}

TEST(VacsNextSectorHandlerTest, MatchSkipsClientsWithEmptyId) {
    const std::vector<Client> clients = {{"", "Approach", "119.800", "EKDK_UC_CTR"}};
    EXPECT_FALSE(MatchVacsClientId("EKDK_UC_CTR", "", clients).has_value());
}

TEST(VacsNextSectorHandlerTest, MatchReturnsNulloptForEmptyTargets) {
    const std::vector<Client> clients = {{"client-1", "Approach", "119.800", "EKDK_UC_CTR"}};
    EXPECT_FALSE(MatchVacsClientId("", "", clients).has_value());
}

TEST(VacsNextSectorHandlerTest, MatchDoesNotLetBlankClientFieldsMatchAnEmptyTarget) {
    const std::vector<Client> clients = {{"client-1", "", "119.800", ""}};
    EXPECT_FALSE(MatchVacsClientId("", "", clients).has_value());
}

// ---------------------------------------------------------------------------
// FormatVacsNextSectorItemString
// ---------------------------------------------------------------------------

TEST(VacsNextSectorHandlerTest, FormatsResolvedSectorAsVPrefixedLabel) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "KA";
    EXPECT_EQ(FormatVacsNextSectorItemString(true, resolution), "V:KA");
}

TEST(VacsNextSectorHandlerTest, FormatsUnresolvedTrackedFlightAsExplicitMarker) {
    EXPECT_EQ(FormatVacsNextSectorItemString(true, {}), "V:--");
}

TEST(VacsNextSectorHandlerTest, FormatsUntrackedFlightAsBlank) {
    EXPECT_EQ(FormatVacsNextSectorItemString(false, {}), "");
}

// ---------------------------------------------------------------------------
// DecideVacsCallTarget (H2: click must match what was painted)
// ---------------------------------------------------------------------------

TEST(VacsNextSectorHandlerTest, DecideDialsWhenPaintedStringMatchesTheFreshResolution) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "KA";
    const auto decision = DecideVacsCallTarget(true, resolution, std::string("client-1"), "V:KA");
    EXPECT_EQ(decision.result, VacsCallGuardResult::Dialable);
    EXPECT_EQ(decision.clientId, "client-1");
}

TEST(VacsNextSectorHandlerTest, DecideReportsNoTargetWhenResolutionIsUnavailable) {
    const auto decision = DecideVacsCallTarget(true, {}, std::nullopt, "V:--");
    EXPECT_EQ(decision.result, VacsCallGuardResult::NoTarget);
}

TEST(VacsNextSectorHandlerTest, DecideReportsNoTargetWhenNoUniqueClientMatched) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "KA";
    const auto decision = DecideVacsCallTarget(true, resolution, std::nullopt, "V:KA");
    EXPECT_EQ(decision.result, VacsCallGuardResult::NoTarget);
}

TEST(VacsNextSectorHandlerTest, DecideReportsDestinationChangedForANullPaintedString) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "KA";
    const auto decision = DecideVacsCallTarget(true, resolution, std::string("client-1"), nullptr);
    EXPECT_EQ(decision.result, VacsCallGuardResult::DestinationChanged);
}

TEST(VacsNextSectorHandlerTest, DecideReportsDestinationChangedForAnEmptyPaintedString) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "KA";
    const auto decision = DecideVacsCallTarget(true, resolution, std::string("client-1"), "");
    EXPECT_EQ(decision.result, VacsCallGuardResult::DestinationChanged);
}

TEST(VacsNextSectorHandlerTest, DecideReportsDestinationChangedWhenPaintedWasTheUnavailableMarker) {
    // A real target resolved fresh, but the tag still showed "V:--" when
    // clicked: the display had not caught up yet, so refuse and ask to retry.
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "KA";
    const auto decision = DecideVacsCallTarget(true, resolution, std::string("client-1"), "V:--");
    EXPECT_EQ(decision.result, VacsCallGuardResult::DestinationChanged);
}

TEST(VacsNextSectorHandlerTest, DecideReportsDestinationChangedWhenTheSectorChangedBetweenPaintAndClick) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "UC";
    const auto decision = DecideVacsCallTarget(true, resolution, std::string("client-1"), "V:KA");
    EXPECT_EQ(decision.result, VacsCallGuardResult::DestinationChanged);
}


TEST(VacsNextSectorHandlerTest, ScanSkipsPlaceholderIdsBeforeTheNextSector) {
    const auto scan = ScanPredictedNextController({"KA", "--", "???", "   ", "UC"}, "KA", 1);
    ASSERT_TRUE(scan.found);
    EXPECT_EQ(scan.controllerId, "UC");
    EXPECT_FALSE(ScanPredictedNextController({"--", "???"}, "KA").found);
}

TEST(VacsNextSectorHandlerTest, ScanRejectsUnknownExitInsteadOfStartingFromNow) {
    EXPECT_FALSE(ScanPredictedNextController({"CTR", "KA", "UC"}, "KA", -1).found);
}

TEST(VacsNextSectorHandlerTest, ResolveUsesFallbackLabelsAndDropsPlaceholderMatchingKeys) {
    for (const auto& id : {"--", "???", "   "}) {
        const auto handoff = ResolveVacsNextSector(true, "EKDK_KA_APP", id, "EKDK_UC_CTR", "", "", false, "", "");
        const auto coordinated = ResolveVacsNextSector(true, "EKDK_KA_APP", "", "", "EKDK_UC_CTR", id, false, "", "");
        const auto predicted = ResolveVacsNextSector(true, "EKDK_KA_APP", "", "", "", "", true, id, "EKDK_UC_CTR");
        for (const auto& resolution : {handoff, coordinated, predicted}) {
            EXPECT_TRUE(resolution.available);
            EXPECT_EQ(resolution.sectorId, "UC");
            EXPECT_TRUE(resolution.positionId.empty());
            EXPECT_EQ(FormatVacsNextSectorItemString(true, resolution), "V:UC");
        }
    }
}

TEST(VacsNextSectorHandlerTest, MatchNeverUsesPlaceholderPositionAsIdentity) {
    const std::vector<Client> clients = {{"unrelated", "Other controller", "119.800", "--"}};
    EXPECT_FALSE(MatchVacsClientId("EKDK_UC_CTR", "--", clients).has_value());
}

TEST(VacsNextSectorHandlerTest, DecideRefusesPlaceholderLabelsEvenIfAClientMatched) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "--";
    EXPECT_EQ(DecideVacsCallTarget(true, resolution, std::string("client-1"), "V:--").result,
              VacsCallGuardResult::UnusableLabel);
}

TEST(VacsNextSectorHandlerTest, DecideReportsUnusableLabelForAnUnchangedOverlongDestination) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "ABCDEFGHIJKLMNOP";
    const auto painted = FormatVacsNextSectorItemString(true, resolution);
    ASSERT_EQ(painted, "V:??");
    const auto decision = DecideVacsCallTarget(true, resolution, std::string("client-1"), painted.c_str());
    EXPECT_EQ(decision.result, VacsCallGuardResult::UnusableLabel);
    EXPECT_TRUE(decision.clientId.empty());
}

TEST(VacsNextSectorHandlerTest, UntrackedFlightCannotPaintOrDialEvenWithAResolvedTarget) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "UC";
    EXPECT_TRUE(FormatVacsNextSectorItemString(false, resolution).empty());
    EXPECT_EQ(DecideVacsCallTarget(false, resolution, std::string("client-1"), "V:UC").result,
              VacsCallGuardResult::NoTarget);
}


TEST(VacsNextSectorHandlerTest, OverlongPaintedLabelCannotDialAfterDestinationChangesToItsPrefix) {
    VacsNextSectorResolution oldResolution;
    oldResolution.available = true;
    oldResolution.sectorId = "ABCDEFGHIJKLMN";
    oldResolution.fullCallsign = "EKDK_OLD_CTR";
    const auto painted = FormatVacsNextSectorItemString(true, oldResolution);
    ASSERT_EQ(painted, "V:??");

    auto freshResolution = oldResolution;
    freshResolution.sectorId = "ABCDEFGHIJKLM";
    freshResolution.fullCallsign = "EKDK_NEW_CTR";
    const auto decision = DecideVacsCallTarget(true, freshResolution, std::string("new-client"), painted.c_str());
    EXPECT_EQ(decision.result, VacsCallGuardResult::DestinationChanged);
    EXPECT_TRUE(decision.clientId.empty());
}

TEST(VacsNextSectorHandlerTest, CompleteThirteenCharacterLabelRemainsDialable) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "ABCDEFGHIJKLM";
    ASSERT_TRUE(IsVacsNextSectorLabelDialable(resolution));
    const auto painted = FormatVacsNextSectorItemString(true, resolution);
    ASSERT_EQ(painted, "V:ABCDEFGHIJKLM");
    ASSERT_EQ(painted.size(), 15u);
    const auto decision = DecideVacsCallTarget(true, resolution, std::string("client-1"), painted.c_str());
    EXPECT_EQ(decision.result, VacsCallGuardResult::Dialable);
    EXPECT_EQ(decision.clientId, "client-1");
}

TEST(VacsNextSectorHandlerTest, PaintAndClickUseTheSameLabelEligibilityDespiteAUniqueLiveMatch) {
    for (const auto& label : {"KA", "ABCDEFGHIJKLM", "ABCDEFGHIJKLMN", "", "--", "??"}) {
        SCOPED_TRACE(label);
        VacsNextSectorResolution resolution;
        resolution.available = true;
        resolution.sectorId = label;
        resolution.fullCallsign = "EKDK_UC_CTR";
        const auto match = MatchVacsClientId(resolution.fullCallsign, "", {{"client-1", "Controller", "119.800", "EKDK_UC_CTR"}});
        ASSERT_TRUE(match.has_value());
        const auto eligible = IsVacsNextSectorLabelDialable(resolution);
        const auto painted = FormatVacsNextSectorItemString(true, resolution);
        const auto decision = DecideVacsCallTarget(true, resolution, match, painted.c_str());
        EXPECT_EQ(eligible, decision.result == VacsCallGuardResult::Dialable);
        if (!eligible) {
            EXPECT_EQ(painted, "V:??");
            EXPECT_EQ(decision.result, VacsCallGuardResult::UnusableLabel);
            EXPECT_TRUE(decision.clientId.empty());
        }
    }
    EXPECT_FALSE(IsVacsNextSectorLabelDialable({}));
}

TEST(VacsNextSectorHandlerTest, UnusableLabelRequestsPickerEvenWithoutAClientMatch) {
    VacsNextSectorResolution resolution;
    resolution.available = true;
    resolution.sectorId = "ABCDEFGHIJKLMN";
    EXPECT_EQ(DecideVacsCallTarget(true, resolution, std::nullopt, "V:??").result,
              VacsCallGuardResult::UnusableLabel);
}
