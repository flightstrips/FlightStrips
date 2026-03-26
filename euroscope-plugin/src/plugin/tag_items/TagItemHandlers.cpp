#include "TagItemHandlers.h"

namespace FlightStrips::TagItems
{
    void TagItemHandlers::Clear()
    {
        _handlers.clear();
    }

    void TagItemHandlers::RegisterHandler(const std::shared_ptr<TagItemHandler>& handler, const int tagItem)
    {
        _handlers[tagItem] = handler;
    }

    void TagItemHandlers::Handle(EuroScopePlugIn::CFlightPlan flightPlan, EuroScopePlugIn::CRadarTarget radarTarget,
                                 int itemCode, int tagData, char sItemString[16], int* pColorCode, COLORREF* pRGB, double* pFontSize) const
    {
        const auto handler = _handlers.find(itemCode);
        if (handler == _handlers.end() || handler->second == nullptr) return;

        handler->second->Handle(flightPlan, radarTarget, itemCode, tagData, sItemString, pColorCode, pRGB, pFontSize);
    }
}
