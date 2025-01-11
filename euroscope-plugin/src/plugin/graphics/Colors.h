//
// Created by fsr19 on 11/01/2025.
//

#pragma once
#include <gdiplus.h>
#include <memory>

namespace FlightStrips::graphics {

    typedef struct Colors
    {
        const std::unique_ptr<const Gdiplus::Brush> whiteBrush =
            std::make_unique<Gdiplus::SolidBrush>(Gdiplus::Color(255, 255, 255));
        const std::unique_ptr<const Gdiplus::Brush> backgroundBrush =
            std::make_unique<Gdiplus::SolidBrush>(Gdiplus::Color(55, 53, 55));
        const std::unique_ptr<const Gdiplus::Brush> headerBrush =
            std::make_unique<Gdiplus::SolidBrush>(Gdiplus::Color(39, 39, 39));
        const std::unique_ptr<const Gdiplus::Brush> greenBrush =
            std::make_unique<Gdiplus::SolidBrush>(Gdiplus::Color(27, 255, 22));
        const std::unique_ptr<const Gdiplus::Brush> redBrush =
            std::make_unique<Gdiplus::SolidBrush>(Gdiplus::Color(244, 58, 58));

        const std::unique_ptr<const Gdiplus::Pen> backgroundPen =
            std::make_unique<Gdiplus::Pen>(Gdiplus::Color(55, 53, 55), 0.75f);
        const std::unique_ptr<const Gdiplus::Pen> buttonPen =
            std::make_unique<Gdiplus::Pen>(Gdiplus::Color(255, 255, 255), 0.5f);
    } Colors;
}
