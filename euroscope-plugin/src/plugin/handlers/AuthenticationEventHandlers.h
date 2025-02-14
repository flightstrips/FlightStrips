#pragma once
#include "AuthenticationEventHandler.h"

namespace FlightStrips::handlers {
    class AuthenticationEventHandlers {
    public:
        void Clear();
        void OnTokenUpdate(const std::string& token) const;
        void RegisterHandler(const std::shared_ptr<AuthenticationEventHandler>& handler);
    private:
        std::list<std::shared_ptr<AuthenticationEventHandler>> m_handlers;

    };
}
