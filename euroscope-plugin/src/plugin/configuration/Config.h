//
// Created by fsr19 on 10/01/2025.
//

#pragma once

#include <tortellini/tortellini.hh>

namespace FlightStrips::configuration {

class Config {
public:
    explicit Config(std::string path);
protected:
    tortellini::ini ini;
    void save() const;
private:
    std::string path;

};

} // FlightStrips

