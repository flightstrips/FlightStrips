//
// Created by fsr19 on 10/01/2025.
//

#include <utility>

#include "config.h"

namespace FlightStrips::configuration {
    void Config::save() const {
        auto out = std::ofstream(this->path, std::ofstream::trunc);
        out << ini;
        out.close();
    }

    Config::Config(std::string path) : path(std::move(path)) {
        auto in = std::ifstream(this->path);
        if (!in.is_open()) {
            return;
        }

        in >> ini;
        in.close();
    }
} // FlightStrips