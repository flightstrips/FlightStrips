#include <gtest/gtest.h>
#include "graphics/InfoPanel.h"
#include "graphics/InfoScreenObjectIds.h"
#include <filesystem>

using namespace FlightStrips;
TEST(VacsPanelTest, MiniIncomingUsesExactSeparateHitboxes) {
    vacs::View view; view.state = vacs::State::Incoming;
    const auto layout = graphics::CalculateVacsLayout(100, 200, true, view);
    ASSERT_EQ(layout.buttons.size(), 2);
    EXPECT_EQ(layout.buttons[0].rect.left, 158); EXPECT_EQ(layout.buttons[0].rect.right, 204);
    EXPECT_EQ(layout.buttons[1].rect.left, 208); EXPECT_EQ(layout.buttons[1].rect.right, 255);
    EXPECT_EQ(layout.buttons[0].rect.top, 217); EXPECT_EQ(layout.buttons[0].rect.bottom, 231);
    EXPECT_LT(layout.status.right, layout.buttons[0].rect.left);
}
TEST(VacsPanelTest, MiniConnectedHasWidePeerAndEnd) {
    vacs::View view; view.state = vacs::State::Connected;
    const auto layout = graphics::CalculateVacsLayout(0, 0, true, view);
    EXPECT_EQ(layout.status.right, 101); EXPECT_EQ(layout.buttons[0].rect.left, 105);
    EXPECT_EQ(layout.buttons[0].objectId, graphics::InfoScreenObjectIds::VacsEnd);
}
TEST(VacsPanelTest, ExpandedSectionAndTotalHeightMatchContract) {
    graphics::InfoPanelData data; data.sessionSelectable = true; data.connected = true;
    EXPECT_EQ(15 + graphics::CalculateInfoPanelContentHeight(data), 293);
    const auto layout = graphics::CalculateVacsLayout(0, 0, false, data.vacsView);
    EXPECT_EQ(layout.enabled.left, 123); EXPECT_EQ(layout.checkbox.top, 18);
    EXPECT_EQ(layout.status.top, 30); EXPECT_EQ(layout.buttons[0].rect.bottom, 62);
    EXPECT_EQ(graphics::CalculateVacsSectionTop(data, {0, 0, 160, 15}), 153);
    data.showStats = false;
    EXPECT_EQ(graphics::CalculateVacsSectionTop(data, {0, 0, 160, 15}), 153);
}

TEST(VacsPanelTest, RenderNativeScalePreviewsWithActualEuroScopeFont) {
    ULONG_PTR token{};
    Gdiplus::GdiplusStartupInput startup;
    ASSERT_EQ(Gdiplus::GdiplusStartup(&token, &startup, nullptr), Gdiplus::Ok);
    bool fontInstalled = false;
    {
        Gdiplus::FontFamily font(L"EuroScope");
        fontInstalled = font.GetLastStatus() == Gdiplus::Ok;
    }
    if (!fontInstalled) {
        Gdiplus::GdiplusShutdown(token);
        GTEST_SKIP() << "Native rendering verification requires the installed EuroScope font.";
    }
    {
        std::filesystem::create_directories("native-preview");
        graphics::InfoPanelData data;
        data.authenticationState = authentication::AUTHENTICATED;
        data.authenticatedName = "Simon Bjerre";
        data.sessionSelectable = true; data.connected = true;
        data.connectionState.callsign = "EKCH_TWR";
        data.connectionState.primary_frequency = "118.100";
        data.connectionState.relevant_airport = "EKCH";
        data.stats.masterCallsign = "EKCH_TWR";
        data.stats.role = websocket::STATE_MASTER;
        data.vacsEnabled = true; data.vacsMiniPreference = true;
        data.vacsView = {vacs::State::Idle, "Ready", "VACS", "Ready to call", "", false, true, 0};
        const CLSID pngEncoder{0x557cf406, 0x1a04, 0x11d3, {0x9a, 0x73, 0x00, 0x00, 0xf8, 0x1e, 0xf3, 0x2e}};
        const auto render = [&](const wchar_t* path, bool mini) {
            const int height = mini ? data.vacsEnabled && data.vacsMiniPreference ? 33 : 15 : 15 + graphics::CalculateInfoPanelContentHeight(data);
            BITMAPINFO info{};
            info.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
            info.bmiHeader.biWidth = 160; info.bmiHeader.biHeight = -height;
            info.bmiHeader.biPlanes = 1; info.bmiHeader.biBitCount = 32;
            HDC dc = CreateCompatibleDC(nullptr);
            void* pixels = nullptr;
            HBITMAP bitmap = CreateDIBSection(dc, &info, DIB_RGB_COLORS, &pixels, nullptr, 0);
            ASSERT_NE(bitmap, nullptr);
            const auto previous = SelectObject(dc, bitmap);
            {
                graphics::Graphics painter;
                graphics::Colors colors;
                painter.SetHandle(dc);
                EXPECT_EQ(painter.FitStringToWidth("Answer", {0, 0, 46, 14}, true), "Answer");
                EXPECT_EQ(painter.FitStringToWidth("Reject", {0, 0, 47, 14}, true), "Reject");
                EXPECT_EQ(painter.FitStringToWidth("APP 2", {0, 0, 38, 16}, true), "APP 2");
                graphics::DrawInfoPanel(nullptr, painter, colors, {0, 0, 160, 15}, data, mini);
            }
            {
                Gdiplus::Bitmap image(bitmap, nullptr);
                EXPECT_EQ(image.GetWidth(), 160); EXPECT_EQ(image.GetHeight(), height);
                EXPECT_EQ(image.Save(path, &pngEncoder), Gdiplus::Ok);
            }
            SelectObject(dc, previous); DeleteObject(bitmap); DeleteDC(dc);
        };
        render(L"native-preview/expanded.png", false);
        render(L"native-preview/mini-ready.png", true);
        data.vacsMiniPreference = false; render(L"native-preview/mini-header.png", true); data.vacsMiniPreference = true;
        data.vacsView = {vacs::State::Incoming, "EKCH_APP (2)", "APP 2", "Two waiting", "incoming", false, true, 2};
        render(L"native-preview/mini-incoming.png", true);
        data.vacsView = {vacs::State::Connected, "EKCH_APP", "EKCH_APP", "Connected", "active", false, true, 0};
        render(L"native-preview/mini-connected.png", true);
        render(L"native-preview/expanded-connected.png", false);
    }
    Gdiplus::GdiplusShutdown(token);
}
