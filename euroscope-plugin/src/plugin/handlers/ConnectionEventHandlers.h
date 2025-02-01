#pragma once
#include "ConnectionEventHandler.h"


namespace FlightStrips::handlers {

class ConnectionEventHandlers {
    public:
        void Clear();
        void OnOnline() const;
        void RegisterHandler(const std::shared_ptr<ConnectionEventHandler>& handler);
    private:
        std::list<std::shared_ptr<ConnectionEventHandler>> m_handlers;
    };

};

// FlightStrip
