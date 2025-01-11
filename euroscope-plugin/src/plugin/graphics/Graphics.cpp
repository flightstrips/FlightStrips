//
// Created by fsr19 on 11/01/2025.
//

#include "Graphics.h"

namespace FlightStrips {
namespace graphics {
    Graphics::Graphics() {
        stringFormat.SetLineAlignment(Gdiplus::StringAlignmentCenter);
    }

    void Graphics::SetHandle(HDC hdc) {
        graphics.reset(Gdiplus::Graphics::FromHDC(hdc));
    }

    void Graphics::FillRect(const Gdiplus::Brush* brush, const RECT &rect) const {
        const auto gdi = ToGdiRect(rect);
        graphics->FillRectangle(brush, gdi);
    }

    void Graphics::DrawRect(const Gdiplus::Pen* pen, const RECT &rect) const {
        const auto gdi = ToGdiRect(rect);
        graphics->DrawRectangle(pen, gdi);
    }

    void Graphics::DrawString(const std::string &text, const RECT &rect, const Gdiplus::Brush *brush, const Gdiplus::StringAlignment alignment) {
        const auto gdi = ToGdiRectF(rect);
        stringFormat.SetAlignment(alignment);
        const std::wstring str = {text.begin(), text.end()};
        graphics->DrawString(str.c_str(), -1, &font, gdi, &stringFormat, brush);
    }

    void Graphics::DrawXButton(const Gdiplus::Pen *pen, const RECT &rect) const {
        graphics->DrawLine(pen, static_cast<INT>(rect.left), static_cast<INT>(rect.top), static_cast<INT>(rect.right), static_cast<INT>(rect.bottom));
        graphics->DrawLine(pen, static_cast<INT>(rect.right), static_cast<INT>(rect.top), static_cast<INT>(rect.left), static_cast<INT>(rect.bottom));
    }

    void Graphics::DrawLineButton(const Gdiplus::Pen *pen, const RECT &rect) const {
        graphics->DrawLine(pen, static_cast<INT>(rect.left), static_cast<INT>(rect.bottom), static_cast<INT>(rect.right), static_cast<INT>(rect.bottom));
    }

    Gdiplus::Rect Graphics::ToGdiRect(const RECT &rect) {
        return { static_cast<INT>(rect.left), static_cast<INT>(rect.top), static_cast<INT>(rect.right - rect.left), static_cast<INT>(rect.bottom - rect.top) };
    }
    Gdiplus::RectF Graphics::ToGdiRectF(const RECT &rect) {
        return { static_cast<Gdiplus::REAL>(rect.left), static_cast<Gdiplus::REAL>(rect.top), static_cast<Gdiplus::REAL>(rect.right - rect.left), static_cast<Gdiplus::REAL>(rect.bottom - rect.top) };
    }

} // graphics
} // FlightStrips