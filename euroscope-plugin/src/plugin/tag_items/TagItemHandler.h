#pragma once

namespace FlightStrips::TagItems
{
    class TagItemHandler
    {
    public:
        virtual ~TagItemHandler() = default;

        virtual void Handle(EuroScopePlugIn::CFlightPlan FlightPlan,
        EuroScopePlugIn::CRadarTarget RadarTarget, int ItemCode, int TagData, char sItemString[16], int *pColorCode,
        COLORREF *pRGB, double *pFontSize) = 0;

    };
}