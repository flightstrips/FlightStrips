//
// Created by fsr19 on 11/01/2025.
//

#pragma once
#include "Colors.h"
#include "Graphics.h"
#include "authentication/AuthenticationService.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips {
    class FlightStripsPlugin;
}

namespace FlightStrips::graphics {
    class InfoScreen : public EuroScopePlugIn::CRadarScreen {
    public:
        explicit InfoScreen(const std::shared_ptr<authentication::AuthenticationService> &authenticationService,
                            const std::shared_ptr<configuration::UserConfig> &config,
                            const std::weak_ptr<websocket::WebSocketService> &webSocketService,
                            FlightStripsPlugin* plugin);


        void OnRefresh(HDC hDC, int Phase) override;

        void OnAsrContentToBeClosed() override;

        void OnMoveScreenObject(int ObjectType, const char *sObjectId, POINT Pt, RECT Area, bool Released) override;

        void OnClickScreenObject(int ObjectType, const char *sObjectId, POINT Pt, RECT Area, int Button) override;

        bool OnCompileCommand(const char * sCommandLine ) override;

    private:
        const int windowId = 1;
        const int minimizeId = 2;
        const int closeId = 3;
        const int authenticationButtonId = 4;
        const int height = 15;
        const int width = 135;

        bool isOpen = true;
        bool isMinimized = false;
        bool canClick = true;

        std::shared_ptr<authentication::AuthenticationService> authService;
        std::shared_ptr<configuration::UserConfig> userConfig;
        std::weak_ptr<websocket::WebSocketService> webSocketService;
        FlightStripsPlugin *m_plugin;

        RECT menubar;
        HDC hdcHandle = nullptr;
        Graphics graphics;
        Colors colors;
    };
}
