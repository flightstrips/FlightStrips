//
// Created by fsr19 on 10/01/2025.
//

#pragma once

#include "tortellini/tortellini.hh"

namespace FlightStrips::configuration {

class Config {
private:
    std::string path;
protected:
    tortellini::ini ini;
    void save() const;
public:
    explicit Config(std::string path);

};

} // FlightStrips

