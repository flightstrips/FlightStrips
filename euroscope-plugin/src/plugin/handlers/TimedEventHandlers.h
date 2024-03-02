#pragma once

#include "TimedEventHandler.h"

namespace FlightStrips::handlers {
    class TimedEventHandlers {

    public:
        void Clear();
        void OnTimer(int time);
        void RegisterHandler(const std::shared_ptr<TimedEventHandler>& handler);

    private:
        std::list<std::shared_ptr<TimedEventHandler>> m_handlers;
    };
}
