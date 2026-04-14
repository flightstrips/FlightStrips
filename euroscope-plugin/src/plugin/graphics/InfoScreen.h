//
// Created by fsr19 on 11/01/2025.
//

#pragma once
#include "Colors.h"
#include "Graphics.h"
#include "InfoPanel.h"
#include "PdcPopup.h"
#include "PdcClearancePopupState.h"
#include "authentication/AuthenticationService.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips {
    class FlightStripsPlugin;
    namespace flightplan {
        class FlightPlanService;
    }
    namespace runway {
        class RunwayService;
    }
}

namespace FlightStrips::graphics {
    class InfoScreen : public EuroScopePlugIn::CRadarScreen {
    public:
        explicit InfoScreen(const std::shared_ptr<authentication::AuthenticationService> &authenticationService,
                            const std::shared_ptr<configuration::UserConfig> &config,
                            const std::weak_ptr<websocket::WebSocketService> &webSocketService,
                            const std::shared_ptr<flightplan::FlightPlanService>& flightPlanService,
                            const std::shared_ptr<runway::RunwayService>& runwayService,
                            std::shared_ptr<PdcClearancePopupState> pdcPopup,
                            FlightStripsPlugin* plugin);


        void OnRefresh(HDC hDC, int Phase) override;

        void OnAsrContentToBeClosed() override;

        void OnMoveScreenObject(int ObjectType, const char *sObjectId, POINT Pt, RECT Area, bool Released) override;

        void OnClickScreenObject(int ObjectType, const char *sObjectId, POINT Pt, RECT Area, int Button) override;

        bool OnCompileCommand(const char * sCommandLine ) override;

    private:
        const int height = 15;
        const int width = 160;

        bool isOpen = true;
        bool isMinimized = false;
        bool canClick = true;
        bool showStats_ = true;

        std::shared_ptr<authentication::AuthenticationService> authService;
        std::shared_ptr<configuration::UserConfig> userConfig;
        std::weak_ptr<websocket::WebSocketService> webSocketService;
        std::weak_ptr<flightplan::FlightPlanService> m_flightPlanService;
        std::weak_ptr<runway::RunwayService> m_runwayService;
        std::shared_ptr<PdcClearancePopupState> m_pdcPopup;
        FlightStripsPlugin *m_plugin;

        RECT menubar;
        HDC hdcHandle = nullptr;
        Graphics graphics;
        Colors colors;

        [[nodiscard]] InfoPanelData BuildInfoPanelData() const;
        [[nodiscard]] std::optional<PdcPopupData> GetPdcPopupData() const;
        bool HandleWindowChromeClick(int objectType);
        bool HandleSessionModeClick(int objectType);
        bool HandlePdcPopupClick(int objectType, POINT pt, RECT area);
        bool HandlePdcFieldClick(int objectType, POINT pt, RECT area);
        void HandleOpenAppClick();
        void HandleAuthenticationClick();
    };
}
