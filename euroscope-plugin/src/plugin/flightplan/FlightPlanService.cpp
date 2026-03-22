#include "FlightPlanService.h"

#include <algorithm>

namespace {
    constexpr auto LOCAL_CDM_OBSERVATION_WINDOW = std::chrono::seconds(15);
    constexpr int LOCAL_CDM_STABLE_POLLS = 3;
}

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

        const auto fp = radarTarget.GetCorrelatedFlightPlan();
        // Treat auto-correlated FPs with no received data (VFR squawk correlation) as no-FP.
        const bool hasFp = fp.IsValid() && fp.GetFlightPlanData().IsReceived();
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
        const auto callsign = std::string(radarTarget.GetCallsign());

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
            std::string(trackPosition.GetSquawk()),
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
                position.m_Latitude, position.m_Longitude,
                trackPosition.GetPressureAltitude()
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
        if (HasActiveLocalCdmObservationWindow(callsign)) {
            ObserveLocalCdmFlightPlan(flightPlan, "controller-assigned-data-update");
        }

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
        ForgetLocalCdmState(callsign);

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
            ForgetLocalCdmState(callsign);

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

    std::string FlightPlanService::GetEstimatedLandingTime(const EuroScopePlugIn::CFlightPlan &flightPlan) {
        time_t rawtime;
        tm ptm;

        time(&rawtime);
        rawtime += flightPlan.GetPositionPredictions().GetPointsNumber() * 60;
        gmtime_s(&ptm, &rawtime);

        return std::format("{:0>2}{:0>2}", ptm.tm_hour, ptm.tm_min);
    }

    void FlightPlanService::OnTimer(int counter) {
        ObserveQueuedLocalCdmRequests();
        PollLocalCdmObservationWindows();

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

    bool FlightPlanService::LocalCdmSnapshot::HasSendableValues() const {
        return !tobt.empty() || !tsat.empty() || !ttot.empty() || !ctot.empty();
    }

    void FlightPlanService::ObserveQueuedLocalCdmRequests() {
        while (const auto callsign = m_flightStripsPlugin->GetNeedsCdmReady()) {
            RefreshLocalCdmObservationWindow(*callsign, "ready-request");

            const auto fp = m_flightStripsPlugin->FlightPlanSelect(callsign->c_str());
            if (!fp.IsValid()) {
                Logger::Warning("Local CDM observation window queued for unknown flight plan {}", *callsign);
                continue;
            }

            ObserveLocalCdmFlightPlan(fp, "ready-request");
        }
    }

    void FlightPlanService::PollLocalCdmObservationWindows() {
        const auto now = std::chrono::steady_clock::now();

        for (auto it = m_localCdmObservationWindows.begin(); it != m_localCdmObservationWindows.end();) {
            const auto callsign = it->first;
            auto& window = it->second;

            if (now >= window.expires_at) {
                Logger::Info("Local CDM observation window expired for {}", callsign);
                it = m_localCdmObservationWindows.erase(it);
                continue;
            }

            const auto fp = m_flightStripsPlugin->FlightPlanSelect(callsign.c_str());
            if (!fp.IsValid()) {
                Logger::Warning("Local CDM observation window removed because flight plan disappeared for {}", callsign);
                it = m_localCdmObservationWindows.erase(it);
                continue;
            }

            const auto snapshot = ObserveLocalCdmFlightPlan(fp, "window-poll");
            if (snapshot.HasSendableValues()) {
                if (window.has_observation && window.last_observed == snapshot) {
                    window.stable_polls++;
                } else {
                    window.last_observed = snapshot;
                    window.has_observation = true;
                    window.stable_polls = 0;
                }

                if (window.stable_polls >= LOCAL_CDM_STABLE_POLLS) {
                    Logger::Info("Local CDM observation window stabilized for {}", callsign);
                    it = m_localCdmObservationWindows.erase(it);
                    continue;
                }
            }

            ++it;
        }
    }

    bool FlightPlanService::HasActiveLocalCdmObservationWindow(const std::string& callsign) const {
        return m_localCdmObservationWindows.contains(callsign);
    }

    void FlightPlanService::RefreshLocalCdmObservationWindow(const std::string& callsign, const std::string& reason) {
        const bool existed = m_localCdmObservationWindows.contains(callsign);
        auto& window = m_localCdmObservationWindows[callsign];
        window.expires_at = std::chrono::steady_clock::now() + LOCAL_CDM_OBSERVATION_WINDOW;
        window.stable_polls = 0;
        if (existed) {
            Logger::Debug("Refreshed local CDM observation window for {} ({})", callsign, reason);
        } else {
            Logger::Info("Tracking local CDM observation window for {} ({})", callsign, reason);
        }
    }

    auto FlightPlanService::ObserveLocalCdmFlightPlan(EuroScopePlugIn::CFlightPlan flightPlan, const std::string& reason)
        -> LocalCdmSnapshot {
        const auto callsign = std::string(flightPlan.GetCallsign());
        const auto snapshot = ParseLocalCdmAnnotation(
            std::string(flightPlan.GetControllerAssignedData().GetFlightStripAnnotation(0)));

        if (!snapshot.HasSendableValues()) {
            return snapshot;
        }

        if (!m_websocketService->CanSendLocalCdmObservation()) {
            Logger::Debug("Skipping local CDM observation send for {} because websocket role is not ready", callsign);
            return snapshot;
        }

        if (const auto emitted = m_lastSentLocalCdm.find(callsign);
            emitted != m_lastSentLocalCdm.end() && emitted->second == snapshot) {
            Logger::Debug("Suppressed duplicate local CDM observation for {} ({})", callsign, reason);
            return snapshot;
        }

        CdmLocalDataEvent event;
        event.callsign = callsign;
        event.source_position = m_flightStripsPlugin->GetConnectionState().callsign;
        event.source_role = m_websocketService->ShouldSend() ? "master" : "slave";
        event.tobt = snapshot.tobt;
        event.tsat = snapshot.tsat;
        event.ttot = snapshot.ttot;
        event.ctot = snapshot.ctot;
        event.asrt = snapshot.asrt;
        event.tsac = snapshot.tsac;
        event.manual_ctot = snapshot.manual_ctot;

        m_websocketService->SendEvent(event);
        m_lastSentLocalCdm[callsign] = snapshot;

        Logger::Info(
            "Sent local CDM observation for {} from {} ({}) reason={} TOBT='{}' TSAT='{}' TTOT='{}'",
            callsign,
            event.source_position,
            event.source_role,
            reason,
            event.tobt,
            event.tsat,
            event.ttot
        );

        return snapshot;
    }

    void FlightPlanService::ForgetLocalCdmState(const std::string& callsign) {
        m_lastSentLocalCdm.erase(callsign);
        m_localCdmObservationWindows.erase(callsign);
    }

    auto FlightPlanService::ParseLocalCdmAnnotation(const std::string& annotation) -> LocalCdmSnapshot {
        LocalCdmSnapshot snapshot;
        const auto fields = SplitSlashFields(annotation);

        if (fields.size() > 0) snapshot.asrt = TrimWhitespace(fields[0]);
        if (fields.size() > 1) snapshot.tsac = TrimWhitespace(fields[1]);
        if (fields.size() > 2) snapshot.tobt = TrimWhitespace(fields[2]);
        if (fields.size() > 3) snapshot.tsat = TrimWhitespace(fields[3]);
        if (fields.size() > 4) snapshot.ttot = TrimWhitespace(fields[4]);
        if (fields.size() > 7) snapshot.manual_ctot = TrimWhitespace(fields[7]);

        return snapshot;
    }

    std::string FlightPlanService::TrimWhitespace(std::string value) {
        const auto first = value.find_first_not_of(" \t\r\n");
        if (first == std::string::npos) {
            return {};
        }

        const auto last = value.find_last_not_of(" \t\r\n");
        return value.substr(first, last - first + 1);
    }

    std::vector<std::string> FlightPlanService::SplitSlashFields(const std::string& value) {
        std::vector<std::string> result;
        size_t start = 0;

        while (true) {
            const auto separator = value.find('/', start);
            result.push_back(value.substr(start, separator == std::string::npos ? std::string::npos : separator - start));
            if (separator == std::string::npos) {
                break;
            }
            start = separator + 1;
        }

        return result;
    }
}
