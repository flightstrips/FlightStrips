#include "Http.h"
#include "Version.h"
#include <curl/curl.h>
#include <fstream>

#include "Logger.hpp"

namespace FlightStrips::http {
    const char* Http::GetUserAgent() {
        static const std::string userAgent = "FlightStripsPlugin/" + std::string(PLUGIN_VERSION);
        return userAgent.c_str();
    }

    HttpResponse Http::PostUrlEncoded(const std::string &url, const std::string &params){
        const auto curl = curl_easy_init();

        std::string resultBuffer;
        curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
        curl_easy_setopt(curl, CURLOPT_POSTFIELDS, params.c_str());
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, WriteCallback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &resultBuffer);
        curl_easy_setopt(curl, CURLOPT_FOLLOWLOCATION, 1L);
        curl_easy_setopt(curl, CURLOPT_USERAGENT, GetUserAgent());

        if (const auto result = curl_easy_perform(curl); result != CURLE_OK) {
            curl_easy_cleanup(curl);
            return {-1,curl_easy_strerror(result)};
        }

        int http_code = 0;
        curl_easy_getinfo (curl, CURLINFO_RESPONSE_CODE, &http_code);
        curl_easy_cleanup(curl);

        return {http_code, resultBuffer};
    }

    HttpResponse Http::Get(const std::string &url) {
        const auto curl = curl_easy_init();

        std::string resultBuffer;
        curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, WriteCallback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &resultBuffer);
        curl_easy_setopt(curl, CURLOPT_FOLLOWLOCATION, 1L);
        curl_easy_setopt(curl, CURLOPT_USERAGENT, GetUserAgent());

        const auto result = curl_easy_perform(curl);

        if (result != CURLE_OK) {
            curl_easy_cleanup(curl);
            return {-1, curl_easy_strerror(result)};
        }

        int http_code = 0;
        curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);
        curl_easy_cleanup(curl);

        return {http_code, resultBuffer};
    }

    size_t Http::WriteCallback(void *contents, size_t size, size_t nmemb, void *userp) {
        static_cast<std::string *>(userp)->append(static_cast<char *>(contents), size * nmemb);
        return size * nmemb;
    }

    size_t Http::WriteFileCallback(void* contents, size_t size, size_t nmemb, void* userp) {
        auto* stream = static_cast<std::ofstream*>(userp);
        stream->write(static_cast<char*>(contents), size * nmemb);
        return size * nmemb;
    }

    bool Http::DownloadFile(const std::string& url, const std::string& outputPath) {
        const auto curl = curl_easy_init();
        if (!curl) {
            return false;
        }

        std::ofstream outFile(outputPath, std::ios::binary);
        if (!outFile.is_open()) {
            curl_easy_cleanup(curl);
            return false;
        }

        curl_easy_setopt(curl, CURLOPT_URL, url.c_str());
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, WriteFileCallback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &outFile);
        curl_easy_setopt(curl, CURLOPT_FOLLOWLOCATION, 1L);
        curl_easy_setopt(curl, CURLOPT_USERAGENT, GetUserAgent());

        const auto result = curl_easy_perform(curl);

        outFile.close();

        if (result != CURLE_OK) {
            curl_easy_cleanup(curl);
            const auto error = curl_easy_strerror(result);
            Logger::Error("Failed to download file from {}: {}", url, error);
            return false;
        }

        int http_code = 0;
        curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_code);
        curl_easy_cleanup(curl);

        return http_code == 200;
    }
}
