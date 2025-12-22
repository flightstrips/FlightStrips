#pragma once
#include <unordered_map>

#include "TagItemHandler.h"
#include "handlers/FlightPlanEventHandler.h"
#include "stands/StandsBootstrapper.h"


namespace FlightStrips::TagItems
{
    class DeIceHandler final : public TagItemHandler, public handlers::FlightPlanEventHandler
    {
    public:
        DeIceHandler(const std::shared_ptr<stands::StandService>& m_stand_service,
            const std::shared_ptr<configuration::AppConfig>& m_app_config)
            : m_standService(m_stand_service),
              m_appConfig(m_app_config)
        {
        }

        void Handle(EuroScopePlugIn::CFlightPlan FlightPlan, EuroScopePlugIn::CRadarTarget RadarTarget, int ItemCode, int TagData, char sItemString[16], int* pColorCode, COLORREF* pRGB, double* pFontSize) override;
        void FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;
        void ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) override {}
        void FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) override {}
    private:
        std::shared_ptr<stands::StandService> m_standService;
        std::shared_ptr<configuration::AppConfig> m_appConfig;

        std::unordered_map<std::string, std::string> m_deiceLookup = {};
    };
}
