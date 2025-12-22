#include "TagItemHandlers.h"

namespace FlightStrips::TagItems
{
    void TagItemHandlers::Clear()
    {
        std::ranges::fill(_handlers, nullptr);
    }

    void TagItemHandlers::RegisterHandler(const std::shared_ptr<TagItemHandler>& handler, const int tagItem)
    {
        const int index = tagItem - 1;
        if (index < 0 || index >= NUMBER_OF_TAG_ITEMS) return;

        _handlers[index] = handler;
    }

    void TagItemHandlers::Handle(EuroScopePlugIn::CFlightPlan flightPlan, EuroScopePlugIn::CRadarTarget radarTarget,
                                 int itemCode, int tagData, char sItemString[16], int* pColorCode, COLORREF* pRGB, double* pFontSize) const
    {
        const auto index = itemCode - 1;
        if (index < 0 || index >= NUMBER_OF_TAG_ITEMS) return;

        const auto handler = _handlers[index];
        if (handler == nullptr) return;

        handler->Handle(flightPlan, radarTarget, itemCode, tagData, sItemString, pColorCode, pRGB, pFontSize);
    }
}
