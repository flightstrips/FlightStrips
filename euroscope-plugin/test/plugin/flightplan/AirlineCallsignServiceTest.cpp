#include <gtest/gtest.h>

#include <filesystem>
#include <fstream>

#include "flightplan/AirlineCallsignService.h"

using FlightStrips::flightplan::AirlineCallsignService;

namespace {
    std::filesystem::path WriteAirlinesFile(const std::string& name, const std::string& contents) {
        const auto path = std::filesystem::temp_directory_path() / name;
        std::ofstream output(path, std::ios::binary | std::ios::trunc);
        output << contents;
        output.close();
        return path;
    }
}

TEST(AirlineCallsignServiceTest, MissingFileKeepsEmptyMap) {
    const auto path = (std::filesystem::temp_directory_path() / "flightstrips-missing-airlines.txt").string();
    std::filesystem::remove(path);

    const AirlineCallsignService service(path);

    EXPECT_EQ(service.Size(), 0u);
    EXPECT_EQ(service.ResolveSpokenCallsign("SAS123", ""), "");
}

TEST(AirlineCallsignServiceTest, ResolvesTelephonyFromAirlineFile) {
    const auto path = WriteAirlinesFile(
        "flightstrips-airlines-basic.txt",
        ";comment\n"
        "SAS\tSCANDINAVIAN AIRLINES SYSTEM\tSCANDINAVIAN\tDENMARK\n"
    );

    const AirlineCallsignService service(path.string());

    EXPECT_EQ(service.Size(), 1u);
    EXPECT_EQ(service.ResolveSpokenCallsign("SAS123", ""), "SCANDINAVIAN");
}

TEST(AirlineCallsignServiceTest, DoesNotResolveTelephonyForNonNumericFourthCharacter) {
    const auto path = WriteAirlinesFile(
        "flightstrips-airlines-nonnumeric.txt",
        "OYF\tFSR AIR\tFSR AIR\tDENMARK\n"
    );

    const AirlineCallsignService service(path.string());

    EXPECT_EQ(service.ResolveSpokenCallsign("OYFSR", ""), "");
}

TEST(AirlineCallsignServiceTest, RemarksOverrideAirlineFile) {
    const auto path = WriteAirlinesFile(
        "flightstrips-airlines-override.txt",
        "TVF\tTRANSAVIA FRANCE\tFRANCE SOLEIL\tFRANCE\n"
    );

    const AirlineCallsignService service(path.string());

    EXPECT_EQ(service.ResolveSpokenCallsign("TVF123", "RMK/TCAS KLM TRANSAVIA VIRTUAL CS=FRANCE SOLEIL=KLM /V/"),
              "FRANCE SOLEIL");
}

TEST(AirlineCallsignServiceTest, ResolvesCallsignBackslashPatternFromRemarks) {
    const auto path = WriteAirlinesFile(
        "flightstrips-airlines-backslash.txt",
        "EAW\tEUROWINGS\tEUROWINGS\tGERMANY\n"
    );

    const AirlineCallsignService service(path.string());

    EXPECT_EQ(
        service.ResolveSpokenCallsign(
            "EAW123",
            "PBN/A1B1D1L1O1S1 DOF/260618 REG/N537SB EET/EGTT0018 OPR/EAW PER/D RMK/TCAS SIMBRIEF CALLSIGN \\ EXECUTIVE /V/"
        ),
        "EXECUTIVE");
}

TEST(AirlineCallsignServiceTest, ResolvesCallsignEqualsPatternFromRemarks) {
    const auto path = WriteAirlinesFile(
        "flightstrips-airlines-equals.txt",
        "TAX\tAIR TAXI\tAIR TAXI\tPORTUGAL\n"
    );

    const AirlineCallsignService service(path.string());

    EXPECT_EQ(
        service.ResolveSpokenCallsign(
            "TAX123",
            "PBN/A1B1C1D1O1S1 DOF/260618 REG/CSTCA EET/GMMM0048 OPR/TAX PER/D CALL RMK/CALLSIGN AIRTAXI=AIRTAXI VIRTUAL AIRLINE WEBSITE=AIRTAXI.PT /V/"
        ),
        "AIRTAXI");
}
