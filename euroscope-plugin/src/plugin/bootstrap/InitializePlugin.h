//
// Created by fsr19 on 19/05/2023.
//

#pragma once

#include "Container.h"

namespace EuroScopePlugIn {
    class CPlugIn;
};

namespace FlightStrips {
    class InitializePlugin {
    public:
        EuroScopePlugIn::CPlugIn* GetPlugin();
        void PostInit(HINSTANCE dllInstance);
        void EuroScopeCleanup();

    private:
        std::shared_ptr<Container> container;
    };

}
