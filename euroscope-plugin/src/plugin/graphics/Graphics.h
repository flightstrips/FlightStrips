//
// Created by fsr19 on 11/01/2025.
//

#pragma once

namespace FlightStrips::graphics {
    class Graphics {
    public:
        Graphics();
        void SetHandle(HDC hdc);

        void FillRect(const Gdiplus::Brush* brush, const RECT& rect) const;
        void DrawRect(const Gdiplus::Pen* pen, const RECT& rect) const;
        void DrawString(const std::string& text, const RECT &rect, const Gdiplus::Brush* brush, Gdiplus::StringAlignment alignment);
        void DrawXButton(const Gdiplus::Pen* pen, const RECT& rect) const;
        void DrawLineButton(const Gdiplus::Pen* pen, const RECT& rect) const;
        void DrawHLine(const Gdiplus::Pen* pen, int x1, int y, int x2) const;
        void DrawVLine(const Gdiplus::Pen* pen, int x, int y1, int y2) const;
        void FillEllipse(const Gdiplus::Brush* brush, const RECT& rect) const;
        [[nodiscard]] std::string FitStringToWidth(const std::string& text, const RECT& rect) const;

    private:
        std::unique_ptr<Gdiplus::Graphics> graphics;
        Gdiplus::FontFamily family = Gdiplus::FontFamily(L"EuroScope");
        Gdiplus::Font font = Gdiplus::Font(&family, 9);
        Gdiplus::StringFormat stringFormat = Gdiplus::StringFormat(Gdiplus::StringFormatFlags::StringFormatFlagsNoClip);

        static Gdiplus::Rect ToGdiRect(const RECT &rect);
        static Gdiplus::RectF ToGdiRectF(const RECT &rect);
        [[nodiscard]] auto MeasureStringWidth(const std::string& text) const -> Gdiplus::REAL;
    };
}
