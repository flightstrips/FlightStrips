//
// Created by fsr19 on 12/01/2025.
//

#pragma once

namespace FlightStrips::http {

  struct HttpResponse {
    int status_code;
    std::string content;
  };


  class Http {
  public:
    static HttpResponse PostUrlEncoded(const std::string& url, const std::string& params);

  private:
    static size_t WriteCallback(void *contents, size_t size, size_t nmemb, void *userp);

  };

}
