#include "FlightPlanService.h"

namespace FlightStrips::flightplan {
    FlightPlanService::FlightPlanService(
        const std::shared_ptr<websocket::WebSocketService> &websocketService,
        const std::shared_ptr<FlightStripsPlugin> &flightStripsPlugin,
        const std::shared_ptr<stands::StandService> &standService) : m_websocketService(websocketService),
                                                                     m_flightStripsPlugin(flightStripsPlugin),
                                                                     m_standService(standService),
                                                                     m_flightPlans({}) {
    }

    void FlightPlanService::RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) {
        const auto position = radarTarget.GetPosition();
        const auto fp = radarTarget.GetCorrelatedFlightPlan();
        if (!position.IsValid() || !fp.IsValid()) return;
        const auto isArrival = strcmp(fp.GetFlightPlanData().GetDestination(),
                                      m_flightStripsPlugin->GetConnectionState().relevant_airport.c_str()) == 0;
        const auto aircraftPosition = position.GetPosition();
        std::string stand;
        // TODO get airport height
        if (!isArrival && position.GetPressureAltitude() < 1000) {
            if (const auto s = m_standService->GetStand(aircraftPosition); s != nullptr) {
                stand = s->GetName();
            }
        }

        FlightPlan plan = {std::string(position.GetSquawk()), stand};
        const auto callsign = std::string(radarTarget.GetCallsign());

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
        if (shouldSendSquawkEvent) {
            m_websocketService->SendEvent(SquawkEvent(callsign, plan.squawk));
        }
        if (shouldSendStandEvent && !plan.stand.empty()) {
            m_websocketService->SendEvent(StandEvent(callsign, plan.stand));
        }

        m_websocketService->SendEvent(PositionEvent(callsign, aircraftPosition.m_Latitude, aircraftPosition.m_Longitude,
                                                    position.GetPressureAltitude()));
    }

    void FlightPlanService::FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        const auto callsign = std::string(flightPlan.GetCallsign());
        const auto relevantAirport = m_flightStripsPlugin->GetConnectionState().relevant_airport;
        const auto stand = m_standService->GetStand(flightPlan.GetControllerAssignedData().GetFlightStripAnnotation(6),
                                                    relevantAirport);
        if (stand != nullptr) {
            FlightPlan plan{{}, stand->GetName()};
            if (const auto [pair, inserted] = this->m_flightPlans.insert({callsign, plan}); !inserted) {
                if (pair->second.stand != plan.stand) {
                    pair->second.stand = plan.stand;
                }
            }
        }
        if (!m_websocketService->ShouldSend()) return;
        const auto flightPlanData = flightPlan.GetFlightPlanData();
        if (!flightPlanData.IsReceived()) return;
        const auto trackPosition = flightPlan.GetFPTrackPosition();
        if (!trackPosition.IsValid()) return;
        const auto position = trackPosition.GetPosition();

        const auto isArrival = strcmp(flightPlan.GetFlightPlanData().GetDestination(), relevantAirport.c_str()) == 0;
        const auto runway = std::string(isArrival
                                            ? flightPlan.GetFlightPlanData().GetArrivalRwy()
                                            : flightPlan.GetFlightPlanData().GetDepartureRwy());
        const auto controllerAssignedData = flightPlan.GetControllerAssignedData();

        const auto event = StripUpdateEvent{
            callsign,
            std::string(flightPlanData.GetOrigin()),
            std::string(flightPlanData.GetDestination()),
            std::string(flightPlanData.GetAlternate()),
            std::string(flightPlanData.GetRoute()),
            std::string(flightPlanData.GetRemarks()),
            runway,
            std::string(trackPosition.GetSquawk()),
            std::string(controllerAssignedData.GetSquawk()),
            std::string(flightPlanData.GetSidName()),
            flightPlan.GetClearenceFlag(),
            std::string(flightPlan.GetGroundState()),
            controllerAssignedData.GetClearedAltitude(),
            controllerAssignedData.GetFinalAltitude(),
            controllerAssignedData.GetAssignedHeading(),
            std::string(flightPlanData.GetAircraftInfo()),
            {flightPlanData.GetAircraftWtc()},
            Position{
                position.m_Latitude, position.m_Longitude,
                trackPosition.GetPressureAltitude()
            },
            stand == nullptr ? "" : stand->GetName(),
            {flightPlanData.GetCommunicationType()},
            flightPlanData.GetCapibilities() == 0 ? "?" : std::string {flightPlanData.GetCapibilities()},
            isArrival ? "" : std::string(flightPlanData.GetEstimatedDepartureTime()),
            isArrival ? GetEstimatedLandingTime(flightPlan) : ""
        };
        m_websocketService->SendEvent(event);
    }

    void FlightPlanService::ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) {
        if (!m_websocketService->ShouldSend()) return;
        const auto callsign = std::string(flightPlan.GetCallsign());

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
                const auto scratch = flightPlan.GetControllerAssignedData().GetScratchPadString();

                if (_strnicmp(scratch, "GRP/S/", 6) != 0) break;

                const auto stand = std::string(scratch).substr(6);
                // We are not validating the stand here!
                FlightPlan plan{{}, stand};

                if (const auto [pair, inserted] = this->m_flightPlans.insert({callsign, plan}); !inserted) {
                    if (pair->second.stand == stand) {
                        break;
                    }
                    pair->second.stand = stand;
                }

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
        if (!m_websocketService->ShouldSend()) return;
        m_websocketService->SendEvent(AircraftDisconnectEvent(std::string(flightPlan.GetCallsign())));
    }

    FlightPlan * FlightPlanService::GetFlightPlan(const std::string &callsign) {
        const auto flightPlan = m_flightPlans.find(callsign);
        if (flightPlan == m_flightPlans.end()) return nullptr;
        return &(flightPlan->second);
    }

    void FlightPlanService::SetStand(const std::string &callsign, const std::string &stand) {
        FlightPlan plan{{}, stand};
        if (const auto [pair, inserted] = this->m_flightPlans.insert({callsign, plan}); !inserted) {
            if (pair->second.stand != plan.stand) {
                pair->second.stand = plan.stand;
            }
        }
    }

    std::string FlightPlanService::GetEstimatedLandingTime(const EuroScopePlugIn::CFlightPlan &flightPlan) {
        time_t rawtime;
        tm ptm;

        time(&rawtime);
        rawtime += flightPlan.GetPositionPredictions().GetPointsNumber() * 60;
        gmtime_s(&ptm, &rawtime);

        return std::format("{:0>2}{:0>2}", ptm.tm_hour, ptm.tm_min);
    }
}
