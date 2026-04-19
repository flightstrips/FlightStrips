#include "FlightPlanService.h"

#include <algorithm>

namespace FlightStrips::flightplan {
    FlightPlanService::FlightPlanService(
        const std::shared_ptr<websocket::WebSocketService> &websocketService,
        const std::shared_ptr<FlightStripsPlugin> &flightStripsPlugin,
        const std::shared_ptr<stands::StandService> &standService,
        const std::shared_ptr<configuration::AppConfig> &appConfig) : m_websocketService(websocketService),
                                                                      m_flightStripsPlugin(flightStripsPlugin),
                                                                      m_standService(standService),
                                                                      m_appConfig(appConfig),
                                                                      m_flightPlans({}) {
    }

    void FlightPlanService::RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget, const bool isRangeOnly) {
        const auto position = radarTarget.GetPosition();
        if (!position.IsValid()) return;

        const auto callsign = std::string(radarTarget.GetCallsign());
        const auto fp = m_flightStripsPlugin->FlightPlanSelect(callsign.c_str());
        const bool hasFp = fp.IsValid();
        const auto isArrival = hasFp
            ? strcmp(fp.GetFlightPlanData().GetDestination(),
                     m_flightStripsPlugin->GetConnectionState().relevant_airport.c_str()) == 0
            : false;

        const auto aircraftPosition = position.GetPosition();
        std::string stand;
        // TODO get airport height
        if (!isArrival && position.GetPressureAltitude() < 1000) {
            if (const auto s = m_standService->GetStand(aircraftPosition); s != nullptr) {
                stand = s->GetName();
            }
        }

        FlightPlan plan = {
            std::string(position.GetSquawk()),
            stand
        };

        if (isRangeOnly) {
            m_rangeTrackedCallsigns.insert(callsign);
        }

        const auto [pair, inserted] = this->m_flightPlans.insert({callsign, plan});
        bool shouldSendSquawkEvent = true;
        bool shouldSendStandEvent = true;

        if (!inserted) {
            if (pair->second.squawk == plan.squawk) {
                shouldSendSquawkEvent = false;
            } else {
                pair->second.squawk = plan.squawk;
            }

            if (plan.stand.empty() || pair->second.stand == plan.stand) {
                shouldSendStandEvent = false;
            } else {
                pair->second.stand = plan.stand;
            }
        }
        if (!m_websocketService->ShouldSend()) return;

        // For no-FP aircraft, on first encounter send a minimal StripUpdateEvent so the backend
        // creates a record. Without this, all subsequent position/squawk events are silently dropped.
        if (inserted && !hasFp) {
            const auto event = StripUpdateEvent{
                callsign,
                "",  // origin — unknown for VFR
                "",  // destination — unknown for VFR
                "", "", "", "",  // alternate, route, remarks, runway
                std::string(position.GetSquawk()), "", "",  // squawk, assigned_squawk, sid
                false, "",   // cleared, ground_state
                0, 0, 0,    // cleared_altitude, requested_altitude, heading
                "", "",     // aircraft_type, aircraft_category
                Position{aircraftPosition.m_Latitude, aircraftPosition.m_Longitude, position.GetPressureAltitude()},
                stand,
                "", "", "",  // communication_type, capabilities, eobt
                "",          // eldt
                "",          // tracking_controller
                "",          // engine_type
                false        // has_fp — no flight plan received
            };
            m_websocketService->SendEvent(event);
        }

        if (shouldSendSquawkEvent) {
            m_websocketService->SendEvent(SquawkEvent(callsign, plan.squawk));
        }
        if (shouldSendStandEvent && !plan.stand.empty()) {
            m_websocketService->SendEvent(StandEvent(callsign, plan.stand));
        }

        // Queue position update instead of sending immediately
        m_pendingPositionUpdates.insert_or_assign(callsign, PositionEvent(callsign, aircraftPosition.m_Latitude,
                                                                          aircraftPosition.m_Longitude,
                                                                          position.GetPressureAltitude()));
    }

    void FlightPlanService::FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        const auto callsign = std::string(flightPlan.GetCallsign());
        const auto relevantAirport = m_flightStripsPlugin->GetConnectionState().relevant_airport;
        auto &plan = this->m_flightPlans.try_emplace(callsign).first->second;

        auto stand = m_standService->GetStand(flightPlan.GetControllerAssignedData().GetFlightStripAnnotation(6),
                                              relevantAirport);
        if (stand != nullptr) {
            plan.stand = stand->GetName();
        }

        if (!m_websocketService->ShouldSend()) return;
        const auto radarTarget = m_flightStripsPlugin->RadarTargetSelect(callsign.c_str());
        const auto radarPosition = radarTarget.GetPosition();
        if (!radarPosition.IsValid()) return;
        const auto position = radarPosition.GetPosition();
        const auto flightPlanData = flightPlan.GetFlightPlanData();

        const auto isArrival = strcmp(flightPlan.GetFlightPlanData().GetDestination(), relevantAirport.c_str()) == 0;
        const auto runway = std::string(isArrival
                                            ? flightPlan.GetFlightPlanData().GetArrivalRwy()
                                            : flightPlan.GetFlightPlanData().GetDepartureRwy());
        const auto controllerAssignedData = flightPlan.GetControllerAssignedData();

        auto standName = stand == nullptr ? "" : stand->GetName();
        if (stand == nullptr) {
            if (const auto fpStand = this->m_flightPlans.find(callsign);
                fpStand != this->m_flightPlans.end() && !fpStand->second.stand.empty()) {
                standName = fpStand->second.stand;
            }
        }

        const auto event = StripUpdateEvent{
            callsign,
            std::string(flightPlanData.GetOrigin()),
            std::string(flightPlanData.GetDestination()),
            std::string(flightPlanData.GetAlternate()),
            std::string(flightPlanData.GetRoute()),
            std::string(flightPlanData.GetRemarks()),
            runway,
            std::string(radarPosition.GetSquawk()),
            std::string(controllerAssignedData.GetSquawk()),
            std::string(flightPlanData.GetSidName()),
            flightPlan.GetClearenceFlag(),
            std::string(flightPlan.GetGroundState()),
            controllerAssignedData.GetClearedAltitude(),
            flightPlanData.GetFinalAltitude(),
            controllerAssignedData.GetAssignedHeading(),
            std::string(flightPlanData.GetAircraftInfo()),
            {flightPlanData.GetAircraftWtc()},
            Position{
                position.m_Latitude, position.m_Longitude, radarPosition.GetPressureAltitude()
            },
            standName,
            {flightPlanData.GetCommunicationType()},
            flightPlanData.GetCapibilities() == 0 ? "?" : std::string{flightPlanData.GetCapibilities()},
            isArrival ? "" : std::string(flightPlanData.GetEstimatedDepartureTime()),
            isArrival ? GetEstimatedLandingTime(flightPlan) : "",
            std::string(flightPlan.GetTrackingControllerCallsign()),
            {flightPlanData.GetEngineType()}
        };
        m_websocketService->SendEvent(event);
    }

    void FlightPlanService::ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) {
        const auto callsign = std::string(flightPlan.GetCallsign());
        if (!m_websocketService->ShouldSend()) return;

        switch (dataType) {
            case EuroScopePlugIn::CTR_DATA_TYPE_SQUAWK:
                m_websocketService->SendEvent(
                    AssignedSquawkEvent(callsign, std::string(flightPlan.GetControllerAssignedData().GetSquawk())));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_FINAL_ALTITUDE:
                m_websocketService->SendEvent(
                    RequestedAltitudeEvent(callsign, flightPlan.GetControllerAssignedData().GetFinalAltitude()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_TEMPORARY_ALTITUDE:
                m_websocketService->SendEvent(
                    ClearedAltitudeEvent(callsign, flightPlan.GetControllerAssignedData().GetClearedAltitude()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_COMMUNICATION_TYPE:
                m_websocketService->SendEvent(
                    CommunicationTypeEvent(callsign, flightPlan.GetControllerAssignedData().GetCommunicationType()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_GROUND_STATE:
                // TODO maybe get the ground state from topsky instead
                m_websocketService->SendEvent(GroundStateEvent(callsign, std::string(flightPlan.GetGroundState())));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_CLEARENCE_FLAG:
                m_websocketService->SendEvent(ClearedFlagEvent(callsign, flightPlan.GetClearenceFlag()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_HEADING:
                m_websocketService->SendEvent(
                    HeadingEvent(callsign, flightPlan.GetControllerAssignedData().GetAssignedHeading()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_SCRATCH_PAD_STRING: {
                auto &plan = this->m_flightPlans.try_emplace(callsign).first->second;
                const auto scratch = flightPlan.GetControllerAssignedData().GetScratchPadString();

                if (_strnicmp(scratch, "GRP/S/", 6) != 0) break;

                const auto stand = std::string(scratch).substr(6);
                // We are not validating the stand here!
                if (plan.stand == stand) {
                    break;
                }
                plan.stand = stand;

                m_websocketService->SendEvent(StandEvent(callsign, stand));
                break;
            }
            case EuroScopePlugIn::CTR_DATA_TYPE_DEPARTURE_SEQUENCE:
                // TODO should we use this???
                break;
            default:
                break;
        }
    }

    void FlightPlanService::FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        const auto callsign = std::string(flightPlan.GetCallsign());

        const bool wasTracked = m_flightPlans.count(callsign) > 0 ||
                                m_rangeTrackedCallsigns.count(callsign) > 0;

        // Remove pending position updates for disconnected aircraft
        m_pendingPositionUpdates.erase(callsign);
        m_flightPlans.erase(callsign);
        m_rangeTrackedCallsigns.erase(callsign);

        if (!wasTracked) return;
        if (!m_websocketService->ShouldSend()) return;
        m_websocketService->SendEvent(AircraftDisconnectEvent(callsign));
    }

    FlightPlan *FlightPlanService::GetFlightPlan(const std::string &callsign) {
        const auto flightPlan = m_flightPlans.find(callsign);
        if (flightPlan == m_flightPlans.end()) return nullptr;
        return &(flightPlan->second);
    }

    void FlightPlanService::RadarTargetOutOfRangeEvent(EuroScopePlugIn::CRadarTarget radarTarget) {
        const auto callsign = std::string(radarTarget.GetCallsign());
        if (m_rangeTrackedCallsigns.find(callsign) == m_rangeTrackedCallsigns.end()) return;

        // Only send disconnect if configured to do so — default is to keep the strip until actual disconnect
        if (m_appConfig->GetDisconnectOnOutOfRange()) {
            m_rangeTrackedCallsigns.erase(callsign);
            m_pendingPositionUpdates.erase(callsign);
            m_flightPlans.erase(callsign);

            if (!m_websocketService->ShouldSend()) return;
            m_websocketService->SendEvent(AircraftDisconnectEvent(callsign));
        }
    }

    void FlightPlanService::SetStand(const std::string &callsign, const std::string &stand) {
        FlightPlan plan{{}, stand};
        if (const auto [pair, inserted] = this->m_flightPlans.insert({callsign, plan}); !inserted) {
            if (pair->second.stand != plan.stand) {
                pair->second.stand = plan.stand;
            }
        }
    }

    void FlightPlanService::ApplyCdmUpdate(const CdmUpdateEvent& event) {
        auto& plan = m_flightPlans.try_emplace(event.callsign).first->second;
        plan.cdm.eobt = event.eobt;
        plan.cdm.tobt = event.tobt;
        plan.cdm.req_tobt = event.req_tobt;
        plan.cdm.req_tobt_source = event.req_tobt_source;
        plan.cdm.tobt_confirmed_by = event.tobt_confirmed_by;
        plan.cdm.tsat = event.tsat;
        plan.cdm.ttot = event.ttot;
        plan.cdm.ctot = event.ctot;
        plan.cdm.asrt = event.asrt;
        plan.cdm.tsac = event.tsac;
        plan.cdm.asat = event.asat;
        plan.cdm.status = event.status;
        plan.cdm.manual_ctot = event.manual_ctot;
        plan.cdm.deice_type = event.deice_type;
        plan.cdm.ecfmp_id = event.ecfmp_id;
        plan.cdm.phase = event.phase;
    }

    void FlightPlanService::ApplyBackendSyncCdm(const std::string& callsign, const BackendSyncCdmData& cdmData) {
        ApplyCdmUpdate(CdmUpdateEvent{
            callsign,
            cdmData.eobt,
            cdmData.tobt,
            cdmData.req_tobt,
            cdmData.req_tobt_source,
            cdmData.tsat,
            cdmData.ttot,
            cdmData.ctot,
            cdmData.asrt,
            cdmData.tsac,
            cdmData.asat,
            cdmData.status,
            cdmData.manual_ctot,
            cdmData.deice_type,
            cdmData.ecfmp_id,
            cdmData.phase,
            cdmData.tobt_confirmed_by
        });
    }

    void FlightPlanService::ApplyPdcStateChange(const std::string& callsign, const std::string& state, const std::string& requestRemarks) {
        auto& plan = m_flightPlans.try_emplace(callsign).first->second;
        plan.pdc_state = state;
        plan.pdc_request_remarks = requestRemarks;
    }

    std::string FlightPlanService::GetEstimatedLandingTime(const EuroScopePlugIn::CFlightPlan &flightPlan) {
        time_t rawtime;
        tm ptm;

        time(&rawtime);
        rawtime += flightPlan.GetPositionPredictions().GetPointsNumber() * 60;
        gmtime_s(&ptm, &rawtime);

        return std::format("{:0>2}{:0>2}", ptm.tm_hour, ptm.tm_min);
    }

    void FlightPlanService::OnTimer(int counter) {
        const auto interval = m_appConfig->GetPositionUpdateIntervalSeconds();

        if (counter - m_lastPositionFlushCounter >= interval) {
            FlushPositionUpdates();
            m_lastPositionFlushCounter = counter;
        }

        // Poll for tracking controller changes every tick.
        // OnFlightPlanControllerAssignedDataUpdate does not fire when a tracking controller is
        // assumed or dropped, so we must detect the transition here instead.
        if (!m_websocketService->ShouldSend()) return;
        for (auto it = m_flightStripsPlugin->FlightPlanSelectFirst(); it.IsValid();
             it = m_flightStripsPlugin->FlightPlanSelectNext(it)) {
            if (it.GetSimulated()) continue;
            const auto callsign = std::string(it.GetCallsign());
            const auto trackingController = std::string(it.GetTrackingControllerCallsign());
            auto &plan = m_flightPlans.try_emplace(callsign).first->second;
            if (plan.tracking_controller == trackingController) continue;
            plan.tracking_controller = trackingController;
            m_websocketService->SendEvent(TrackingControllerChangedEvent(callsign, trackingController));
        }
    }

    void FlightPlanService::FlushPositionUpdates() {
        if (!m_websocketService->ShouldSend() || m_pendingPositionUpdates.empty()) return;

        for (const auto& [callsign, positionEvent] : m_pendingPositionUpdates) {
            m_websocketService->SendEvent(positionEvent);
        }

        m_pendingPositionUpdates.clear();
    }
}
