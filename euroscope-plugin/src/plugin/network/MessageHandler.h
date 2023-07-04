#pragma once

namespace FlightStrips {
    class FlightStripsPlugin;
}

namespace FlightStrips::network {
    class MessageHandler {
    public:
        explicit MessageHandler(const std::shared_ptr<FlightStripsPlugin> &mPlugin);

        void OnMessage(const std::string& string);

    private:
       std::shared_ptr<FlightStripsPlugin> m_plugin;

    };
}
