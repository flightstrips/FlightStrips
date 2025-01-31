#include "AuthenticationService.h"

#include <format>
#include <openssl/rand.h>
#include <openssl/sha.h>
#include <nlohmann/json.hpp>
#include "AuthenticationRedirectListener.h"
#include "Logger.h"
#include "http/Http.h"

namespace FlightStrips::authentication {
    AuthenticationService::AuthenticationService(const std::shared_ptr<configuration::AppConfig> &appConfig,
                                                 const std::shared_ptr<configuration::UserConfig> &
                                                 userConfig) : appConfig(appConfig), userConfig(userConfig) {
        LoadFromConfig();
    }

    AuthenticationService::~AuthenticationService() {
        CancelAuthentication();

        // Cleanup
        if (token_thread.joinable()) {
            token_thread.join();
        }
    }

    void AuthenticationService::Logout() {
        accessToken = "";
        refreshToken = "";
        expirationTime = 0;
        name = "";
        state = NONE;
        userConfig->SetToken({});
    }

    void AuthenticationService::StartAuthentication() {
        if (state != NONE) {
            return;
        }

        if (token_thread.joinable()) {
            token_thread.join();
        }

        state = true;
        this->token_thread = std::thread(&AuthenticationService::DoAuthenticationFlow, this);
    }

    void AuthenticationService::CancelAuthentication() {
        if (state != LOGIN) return;
        state = NONE;
        if (token_thread.joinable()) {
            token_thread.join();
        }
    }

    AuthenticationState AuthenticationService::GetAuthenticationState() const {
        return static_cast<AuthenticationState>(state.load());
    }

    std::string AuthenticationService::GetName() const {
        return name;
    }

    std::string AuthenticationService::GetAccessToken() const {
        return accessToken;
    }

    void AuthenticationService::OnTimer(int time) {
        if (state == AUTHENTICATED && NeedsRefresh()) {
            StartRefresh();
        }
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

    std::optional<nlohmann::json> AuthenticationService::GetTokenPayload(const std::string &access_token) {
        const auto start = access_token.find('.');
        if (start == std::string::npos) {
            Logger::Error("GetTokenPayload start '.' not found");
            return {};
        }
        const auto end = access_token.find('.', start + 1);
        if (end == std::string::npos) {
            Logger::Error("GetTokenPayload end '.' not found");
            return {};
        }

        const std::string token = access_token.substr(start + 1, end - (start + 1));
        const auto decoded = base64_decode(token);
        try {
            return nlohmann::json::parse(decoded);
        } catch (const std::exception &e) {
            Logger::Error(std::format("Failed to parse payload part of token: {}", e.what()));
            return {};
        }
    }

    void AuthenticationService::LoadFromConfig() {
        auto token = this->userConfig->GetToken();
        accessToken = token.accessToken;
        refreshToken = token.refreshToken;
        expirationTime = token.expiry;

        if (accessToken.empty() || refreshToken.empty()) return;

        const auto parsed_id_token = GetTokenPayload(token.idToken);
        if (parsed_id_token.has_value()) {
            name = parsed_id_token.value()["name"];
        }

        state = AUTHENTICATED;
        Logger::Debug(std::format("Name: {}", name));
    }

    bool AuthenticationService::NeedsRefresh() const {
        const time_t now = std::chrono::system_clock::to_time_t(std::chrono::system_clock::now());
        return expirationTime < now + 60 * 30;
    }

    void AuthenticationService::StartRefresh() {
        if (state != AUTHENTICATED) return;
        state = REFRESH;

        if (token_thread.joinable()) {
            token_thread.join();
        }

        token_thread = std::thread(&AuthenticationService::DoRefreshFlow, this);
    }

    void AuthenticationService::DoRefreshFlow() {
        Logger::Debug(std::format("Refreshing authentication token."));
        const auto token_url = std::format("{}/oauth/token", this->appConfig->GetAuthority());
        std::ostringstream token_params;

        token_params << "grant_type=refresh_token"
                << "&client_id=" << this->appConfig->GetClientId()
                << "&refresh_token=" << refreshToken;

        const auto [status_code, content] = http::Http::PostUrlEncoded(token_url, token_params.str());
        if (status_code != 200) {
            Logger::Error(std::format("Failed to refresh token. HTTP response code: {}. Content: {}", status_code,
                                      content));
            Logout();
            return;
        }

        if (!ParseAndSetToken(content)) {
            Logout();
            return;
        }

        Logger::Debug(std::format("Refreshing authentication token completed."));
    }

    bool AuthenticationService::ParseAndSetToken(const std::string &content) {
        nlohmann::json token_json;
        try {
            token_json = nlohmann::json::parse(content);
        } catch (const std::exception &e) {
            Logger::Error(std::format("Failed to parse authentication token endpoint JSON. Error: {}", e.what()));
            return false;
        }

        std::string access_token = token_json["access_token"];
        std::string refresh_token = refreshToken;
        if (token_json.contains("refresh_token")) {
            refresh_token = token_json["refresh_token"];
        }
        std::string id_token = token_json["id_token"];
        auto access_token_payload = GetTokenPayload(access_token);
        auto id_token_payload = GetTokenPayload(id_token);
        if (!access_token_payload.has_value() || !id_token_payload.has_value()) {
            return false;
        }

        const int exp = access_token_payload.value()["exp"];

        // TODO event
        configuration::Token token = {access_token, refresh_token, id_token, exp};
        userConfig->SetToken(token);

        this->accessToken = access_token;
        this->refreshToken = refresh_token;
        this->name = id_token_payload.value()["name"];
        this->expirationTime = exp;
        state = AUTHENTICATED;

        return true;
    }

    void AuthenticationService::DoAuthenticationFlowImpl() {
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

        if (!state) return;

        auto code = code_result.value();
        Logger::Debug(std::format("Authorization code: {}", code));

        auto token_result = GetTokenFromAuthorizationCode(client_id, code_verifier, code, redirect_uri);
        Logger::Debug(std::format("Got authentication token: {}", token_result));

        ParseAndSetToken(token_result);
    }

    void AuthenticationService::DoAuthenticationFlow() {
        DoAuthenticationFlowImpl();
        if (state == AUTHENTICATED) return;
        state = NONE;
    }

    bool AuthenticationService::WaitForResult(const std::future<std::optional<std::string> > &future) const {
        auto result = future.wait_for(std::chrono::milliseconds(10));
        while (result == std::future_status::timeout && this->state == LOGIN) {
            result = future.wait_for(std::chrono::milliseconds(10));
        }

        return result == std::future_status::ready;
    }

    std::string AuthenticationService::GetTokenFromAuthorizationCode(const std::string &clientId,
                                                                     const std::string &codeVerifier,
                                                                     const std::string &code,
                                                                     const std::string &redirectUrl) const {
        const auto token_url = std::format("{}/oauth/token", this->appConfig->GetAuthority());

        std::ostringstream token_params;

        token_params << "grant_type=authorization_code"
                << "&client_id=" << clientId
                << "&code_verifier=" << codeVerifier
                << "&code=" << code
                << "&redirect_uri=" << redirectUrl;

        const auto [status_code, content] = http::Http::PostUrlEncoded(token_url, token_params.str());
        return content;
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

    std::string AuthenticationService::base64_decode(const std::string &in) {
        std::string out;
        std::vector<int> T(256, -1);
        unsigned int i;
        for (i = 0; i < 64; i++) T[base64_url_alphabet[i]] = i;

        int val = 0, valb = -8;
        for (i = 0; i < in.length(); i++) {
            unsigned char c = in[i];
            if (T[c] == -1) break;
            val = (val << 6) + T[c];
            valb += 6;
            if (valb >= 0) {
                out.push_back(char((val >> valb) & 0xFF));
                valb -= 8;
            }
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
