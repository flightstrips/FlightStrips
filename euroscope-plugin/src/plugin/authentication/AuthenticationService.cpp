#include "AuthenticationService.h"

#include <format>
#include <openssl/rand.h>
#include <openssl/sha.h>
#include <curl/curl.h>

#include "AuthenticationRedirectListener.h"
#include "Logger.h"

namespace FlightStrips::authentication {
    AuthenticationService::AuthenticationService(const std::shared_ptr<configuration::AppConfig> &appConfig,
                                                 const std::shared_ptr<configuration::UserConfig> &
                                                 userConfig) : appConfig(appConfig), userConfig(userConfig) {
    }

    AuthenticationService::~AuthenticationService() {
        CancelAuthentication();
    }

    void AuthenticationService::StartAuthentication() {
        CancelAuthentication();

        running_token = true;
        this->token_thread = std::thread(&AuthenticationService::DoAuthenticationFlow, this);
    }

    void AuthenticationService::CancelAuthentication() {
        if (!running_token) return;
        running_token = false;
        if (token_thread.joinable()) {
            token_thread.join();
        }
    }

    bool AuthenticationService::IsRunningAuthentication() const {
        return running_token;
    }

    std::string AuthenticationService::GetAuthorizeUrl(const std::string &code_challenge, const std::string &client_id,
                                                       const std::string &redirect_uri) const {
        std::ostringstream url;

        const std::string scope = this->appConfig->GetScopes();
        // TODO trim end '/'
        const std::string auth_endpoint = std::format("{}/authorize", this->appConfig->GetAuthority());

        url << auth_endpoint
                << "?response_type=code"
                << "&client_id=" << client_id
                << "&redirect_uri=" << redirect_uri
                << "&scope=" << scope
                << "&code_challenge=" << code_challenge
                << "&code_challenge_method=S256"
                << "&audience=backend";

        return url.str();
    }

    void AuthenticationService::OpenBrowser(const std::string &url) {
        Logger::Debug(std::format("Opening browser with url: {}", url));
        const auto wurl = std::wstring(url.begin(), url.end());
        ShellExecute(nullptr, nullptr, wurl.c_str(), nullptr, nullptr, SW_SHOW);
    }

    void AuthenticationService::DoAuthenticationFlow() {
        Logger::Debug("Starting authentication flow");
        std::promise<std::optional<std::string> > promise;
        std::future<std::optional<std::string> > future = promise.get_future();

        AuthenticationRedirectListener listener(promise, this->appConfig->GetRedirectPort());
        listener.Start();

        const auto code_verifier = generateCodeVerifier();
        const auto code_challenge = generateCodeChallenge(code_verifier);
        const std::string client_id = this->appConfig->GetClientId();
        const std::string redirect_uri = std::format("http://127.0.0.1:{}/callback-auth0",
                                                     this->appConfig->GetRedirectPort());
        auto url = GetAuthorizeUrl(code_challenge, client_id, redirect_uri);
        OpenBrowser(url);

        const auto ready = WaitForResult(future);
        listener.Stop();

        if (!ready) {
            Logger::Debug("Authentication cancelled");
            return;
        }

        auto code_result = future.get();
        if (!code_result.has_value()) {
            Logger::Debug("Authentication failed, got no authorization code.");
            return;
        }

        if (!running_token) return;

        auto code = code_result.value();
        Logger::Debug(std::format("Authorization code: {}", code));

        auto token_result = GetTokenFromAuthorizationCode(client_id, code_verifier, code, redirect_uri);
        Logger::Debug(std::format("Got authentication token: {}", token_result));
    }

    bool AuthenticationService::WaitForResult(const std::future<std::optional<std::string> > &future) const {
        auto result = future.wait_for(std::chrono::milliseconds(10));
        while (result == std::future_status::timeout && this->running_token) {
            result = future.wait_for(std::chrono::milliseconds(10));
        }

        return result == std::future_status::ready;
    }

    std::string AuthenticationService::GetTokenFromAuthorizationCode(const std::string &clientId,
                                                                     const std::string &codeVerifier,
                                                                     const std::string &code,
                                                                     const std::string &redirectUrl) const {
        const auto curl = curl_easy_init();
        const auto token_url = std::format("{}/oauth/token", this->appConfig->GetAuthority());

        std::ostringstream token_params;

        token_params << "grant_type=authorization_code"
                << "&client_id=" << clientId
                << "&code_verifier=" << codeVerifier
                << "&code=" << code
                << "&redirect_uri=" << redirectUrl;

        std::string resultBuffer;
        curl_easy_setopt(curl, CURLOPT_URL, token_url);
        curl_easy_setopt(curl, CURLOPT_POSTFIELDS, token_params.str());
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, WriteCallback);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, &resultBuffer);
        curl_easy_setopt(curl, CURLOPT_FOLLOWLOCATION, 1L);

        const auto result = curl_easy_perform(curl);

        curl_easy_cleanup(curl);

        if (result != CURLE_OK) {
            return curl_easy_strerror(result);
        }

        return resultBuffer;
    }

    size_t AuthenticationService::WriteCallback(void *contents, size_t size, size_t nmemb, void *userp) {
        static_cast<std::string *>(userp)->append(static_cast<char *>(contents), size * nmemb);
        return size * nmemb;
    }

    std::string AuthenticationService::generateCodeVerifier() {
        constexpr int length = 64; // Recommended length for code verifier
        std::vector<unsigned char> buffer(length);

        if (RAND_bytes(buffer.data(), length) != 1) {
            throw std::runtime_error("RAND_bytes failed: Insufficient entropy or RNG issue");
        }

        // Base64 URL-encode the random bytes
        return base64_encode({buffer.begin(), buffer.end()});
    }

    std::string AuthenticationService::base64_encode(const std::string &in) {
        std::string out;
        int val = 0, valb = -6;
        size_t len = in.length();
        unsigned int i = 0;
        for (i = 0; i < len; i++) {
            unsigned char c = in[i];
            val = (val << 8) + c;
            valb += 8;
            while (valb >= 0) {
                out.push_back(base64_url_alphabet[(val >> valb) & 0x3F]);
                valb -= 6;
            }
        }
        if (valb > -6) {
            out.push_back(base64_url_alphabet[((val << 8) >> (valb + 8)) & 0x3F]);
        }
        return out;
    }

    std::string AuthenticationService::generateCodeChallenge(const std::string &codeVerifier) {
        unsigned char hash[SHA256_DIGEST_LENGTH];
        SHA256(reinterpret_cast<const unsigned char *>(codeVerifier.c_str()), codeVerifier.size(), hash);

        // Base64 URL-encode the hash
        return base64_encode(std::string(reinterpret_cast<char *>(hash), SHA256_DIGEST_LENGTH));
    }
}
