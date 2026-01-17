#pragma once

namespace FlightStrips::http {

  struct HttpResponse {
    int status_code;
    std::string content;
  };


  class Http {
  public:
    static HttpResponse PostUrlEncoded(const std::string& url, const std::string& params);
    static HttpResponse Get(const std::string& url);
    static bool DownloadFile(const std::string& url, const std::string& outputPath);

  private:
    static const char* GetUserAgent();
    static size_t WriteCallback(void *contents, size_t size, size_t nmemb, void *userp);
    static size_t WriteFileCallback(void* contents, size_t size, size_t nmemb, void* userp);

  };

}
