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

    void Graphics::DrawHLine(const Gdiplus::Pen *pen, const int x1, const int y, const int x2) const {
        graphics->DrawLine(pen, x1, y, x2, y);
    }

    void Graphics::DrawVLine(const Gdiplus::Pen *pen, const int x, const int y1, const int y2) const {
        graphics->DrawLine(pen, x, y1, x, y2);
    }

    void Graphics::FillEllipse(const Gdiplus::Brush *brush, const RECT &rect) const {
        const auto gdi = ToGdiRect(rect);
        graphics->FillEllipse(brush, gdi);
    }

    std::string Graphics::FitStringToWidth(const std::string& text, const RECT& rect) const {
        const auto maxWidth = static_cast<Gdiplus::REAL>(rect.right - rect.left);
        if (text.empty() || maxWidth <= 0 || MeasureStringWidth(text) <= maxWidth) {
            return text;
        }

        constexpr std::string_view ellipsis = "...";
        if (MeasureStringWidth(std::string(ellipsis)) > maxWidth) {
            return {};
        }

        int low = 0;
        int high = static_cast<int>(text.size());
        while (low < high) {
            const int mid = (low + high + 1) / 2;
            const auto candidate = text.substr(0, mid) + std::string(ellipsis);
            if (MeasureStringWidth(candidate) <= maxWidth) {
                low = mid;
            } else {
                high = mid - 1;
            }
        }

        return low > 0 ? text.substr(0, low) + std::string(ellipsis) : std::string(ellipsis);
    }

    Gdiplus::Rect Graphics::ToGdiRect(const RECT &rect) {
        return { static_cast<INT>(rect.left), static_cast<INT>(rect.top), static_cast<INT>(rect.right - rect.left), static_cast<INT>(rect.bottom - rect.top) };
    }
    Gdiplus::RectF Graphics::ToGdiRectF(const RECT &rect) {
        return { static_cast<Gdiplus::REAL>(rect.left), static_cast<Gdiplus::REAL>(rect.top), static_cast<Gdiplus::REAL>(rect.right - rect.left), static_cast<Gdiplus::REAL>(rect.bottom - rect.top) };
    }

    auto Graphics::MeasureStringWidth(const std::string& text) const -> Gdiplus::REAL {
        const std::wstring wideText(text.begin(), text.end());
        Gdiplus::RectF bounds{};
        Gdiplus::StringFormat format(Gdiplus::StringFormatFlags::StringFormatFlagsNoClip);
        graphics->MeasureString(wideText.c_str(), -1, &font, Gdiplus::PointF(0, 0), &format, &bounds);
        return bounds.Width;
    }

} // graphics
} // FlightStrips
