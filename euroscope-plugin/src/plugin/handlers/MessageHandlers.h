#pragma once
#include "MessageHandler.h"

namespace FlightStrips::handlers {

    class MessageHandlers {
    public:
        void Clear();
        void OnMessages(const std::vector<std::string>& messages) const;
        void RegisterHandler(const std::shared_ptr<MessageHandler>& handler);
    private:
        std::list<std::shared_ptr<MessageHandler>> m_handlers;
    };

}
