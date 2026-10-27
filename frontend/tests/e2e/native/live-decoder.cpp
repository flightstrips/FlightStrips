#include "aman/AMANGainLossStore.h"
#include <fstream>
#include <iostream>
#include <iterator>
#include <stdexcept>

using FlightStrips::aman::AMANGainLossStore;

static void Require(bool condition, const char* message) {
    if (!condition) throw std::runtime_error(message);
}
static std::string Read(const char* path) {
    std::ifstream file(path, std::ios::binary);
    Require(file.is_open(), "cannot open captured protobuf envelope");
    return {std::istreambuf_iterator<char>(file), std::istreambuf_iterator<char>()};
}
static void HealthyNeighbour(AMANGainLossStore& store, const char* callsign) {
    const auto value = store.FindByCallsign(callsign);
    Require(value.has_value(), "healthy neighbour absent from C++ decoder");
    Require(value->seconds.has_value() && value->referencePoint.has_value() && value->targetTime.has_value(),
            "healthy neighbour lost gain/loss guidance in C++ decoder");
    Require(value->dataStatus == "fresh", "healthy neighbour is not fresh");
}
int main(int argc, char** argv) {
    try {
        Require(argc == 4, "expected healthy, unreliable and recovered captured envelope paths");
        AMANGainLossStore store;
        store.SessionChanged("AMAN browser E2E");
        const auto healthy = Read(argv[1]);
        store.OnMessages({healthy});
        Require(store.Snapshot()->hasRevision, "production decoder rejected healthy envelope");
        HealthyNeighbour(store, "SAS101");
        HealthyNeighbour(store, "SAS202");
        HealthyNeighbour(store, "CCA877Q");
        store.OnMessages({Read(argv[2])});
        HealthyNeighbour(store, "SAS101");
        HealthyNeighbour(store, "SAS202");
        const auto affected = store.FindByCallsign("CCA877Q");
        Require(affected.has_value() && !affected->seconds.has_value(), "unreliable aircraft retained fresh native guidance");
        store.OnMessages({Read(argv[3])});
        HealthyNeighbour(store, "CCA877Q");
        const auto recoveredRevision = store.Snapshot()->revision;
        store.OnMessages({healthy});
        Require(store.Snapshot()->revision == recoveredRevision, "old native revision replaced recovery");
        store.Online();
        Require(store.FindByCallsign("SAS101")->dataStatus == "disconnected", "native reconnect lost stale marker");
        store.OnMessages({healthy});
        HealthyNeighbour(store, "SAS101");
        std::cout << "Production C++ AMANGainLossStore accepted live healthy, unreliable, recovered and reconnect envelopes\n";
        return 0;
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n';
        return 1;
    }
}
