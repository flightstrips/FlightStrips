#pragma once

#include <string>
#include <vector>
#include <nlohmann/json.hpp>

namespace FlightStrips::flightplan {
    struct EcfmpRestriction {
        int64_t measure_id{};
        std::string ident{};
        std::string type{};
        std::string reason{};
        std::vector<std::string> routes{};
        std::string destination{};
        int max_level{};
        int min_level{};
        std::vector<int> exact_levels{};
        bool has_ctot{};
    };

    inline void to_json(nlohmann::json& j, const EcfmpRestriction& r) {
        j = nlohmann::json::object();
        if (r.measure_id != 0) j["measure_id"] = r.measure_id;
        if (!r.ident.empty()) j["ident"] = r.ident;
        j["type"] = r.type;
        if (!r.reason.empty()) j["reason"] = r.reason;
        if (!r.routes.empty()) j["routes"] = r.routes;
        if (!r.destination.empty()) j["destination"] = r.destination;
        if (r.max_level != 0) j["max_level"] = r.max_level;
        if (r.min_level != 0) j["min_level"] = r.min_level;
        if (!r.exact_levels.empty()) j["exact_levels"] = r.exact_levels;
        if (r.has_ctot) j["has_ctot"] = r.has_ctot;
    }

    inline void from_json(const nlohmann::json& j, EcfmpRestriction& r) {
        if (j.contains("measure_id") && j["measure_id"].is_number()) r.measure_id = j["measure_id"].get<int64_t>();
        if (j.contains("ident") && j["ident"].is_string()) r.ident = j["ident"].get<std::string>();
        if (j.contains("type") && j["type"].is_string()) r.type = j["type"].get<std::string>();
        if (j.contains("reason") && j["reason"].is_string()) r.reason = j["reason"].get<std::string>();
        if (j.contains("routes") && j["routes"].is_array()) {
            for (const auto& item : j["routes"]) {
                if (item.is_string()) r.routes.push_back(item.get<std::string>());
            }
        }
        if (j.contains("destination") && j["destination"].is_string()) r.destination = j["destination"].get<std::string>();
        if (j.contains("max_level") && j["max_level"].is_number()) r.max_level = j["max_level"].get<int>();
        if (j.contains("min_level") && j["min_level"].is_number()) r.min_level = j["min_level"].get<int>();
        if (j.contains("exact_levels") && j["exact_levels"].is_array()) {
            for (const auto& item : j["exact_levels"]) {
                if (item.is_number()) r.exact_levels.push_back(item.get<int>());
            }
        }
        if (j.contains("has_ctot") && j["has_ctot"].is_boolean()) r.has_ctot = j["has_ctot"].get<bool>();
    }

    struct CdmState {
        std::string eobt{};
        std::string tobt{};
        std::string req_tobt{};
        std::string req_tobt_source{};
        std::string tobt_confirmed_by{};
        std::string tsat{};
        std::string ttot{};
        std::string ctot{};
        std::string asrt{};
        std::string tsac{};
        std::string asat{};
        std::string status{};
        std::string manual_ctot{};
        std::string deice_type{};
        std::string ecfmp_id{};
        std::string phase{};
        std::vector<EcfmpRestriction> ecfmp_restrictions;
    };

    struct FlightPlan {
    public:
        std::string squawk{};
        std::string stand{};
        std::string tracking_controller{};
        std::string runway{};
        bool runway_initialized{false};
        bool strip_synchronized{false};
        CdmState cdm{};
        std::string pdc_state{};
        std::string pdc_request_remarks{};

        [[nodiscard]] bool IsPdcCleared() const {
            return pdc_state == "CLEARED";
        }

        [[nodiscard]] bool IsPdcConfirmed() const {
            return pdc_state == "CONFIRMED";
        }

        [[nodiscard]] bool KeepsEuroScopeStripUncleared() const {
            return IsPdcCleared();
        }

        [[nodiscard]] bool HasRunwayChanged(const std::string& current_runway) const {
            return runway_initialized && runway != current_runway;
        }

        void MarkRunwaySynced(const std::string& synced_runway) {
            runway = synced_runway;
            runway_initialized = true;
        }
    };
}
