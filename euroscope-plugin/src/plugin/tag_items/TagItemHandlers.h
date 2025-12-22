#pragma once
#include "Constants.h"
#include "TagItemHandler.h"


namespace FlightStrips::TagItems
{
    class TagItemHandlers
    {
    public:
        void Clear();
        void RegisterHandler(const std::shared_ptr<TagItemHandler>& handler, const int tagItem);
        void Handle(EuroScopePlugIn::CFlightPlan flightPlan,
                    EuroScopePlugIn::CRadarTarget radarTarget, int itemCode, int tagData, char sItemString[16], int *pColorCode,
                    COLORREF *pRGB, double *pFontSize) const;
    private:
        std::shared_ptr<TagItemHandler> _handlers[NUMBER_OF_TAG_ITEMS];
    };
}

