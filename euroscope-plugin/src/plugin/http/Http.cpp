//
// Created by fsr19 on 12/01/2025.
//

#include "Http.h"
#include <curl/curl.h>

namespace FlightStrips::http {
    HttpResponse Http::PostUrlEncoded(const std::string &url, const std::string &params){
        const auto curl = curl_easy_init();

        std::string resultBuffer;
        curl_easy_setopt(curl, CURLOPT_URL, url);
        curl_easy_setopt(curl, CURLOPT_POSTFIELDS, params);
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, WriteCallback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &resultBuffer);
        curl_easy_setopt(curl, CURLOPT_FOLLOWLOCATION, 1L);

        const auto result = curl_easy_perform(curl);

        if (result != CURLE_OK) {
            curl_easy_cleanup(curl);
            return {-1,curl_easy_strerror(result)};
        }

        int http_code = 0;
        curl_easy_getinfo (curl, CURLINFO_RESPONSE_CODE, &http_code);
        curl_easy_cleanup(curl);

        return {http_code, resultBuffer};
    }

    size_t Http::WriteCallback(void *contents, size_t size, size_t nmemb, void *userp) {
        static_cast<std::string *>(userp)->append(static_cast<char *>(contents), size * nmemb);
        return size * nmemb;
    }
}
