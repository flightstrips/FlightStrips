#pragma once
#include <future>
#include <optional>

#include "configuration/AppConfig.h"
#include "configuration/UserConfig.h"

namespace FlightStrips::authentication {

static constexpr char base64_url_alphabet[] = {
    'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
    'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
    'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
    'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
    '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-', '_'
};

class AuthenticationService {
public:
    AuthenticationService(const std::shared_ptr<configuration::AppConfig> &appConfig, const std::shared_ptr<configuration::UserConfig> &userConfig);
    ~AuthenticationService();

    void StartAuthentication();
    void CancelAuthentication();
    [[nodiscard]] bool IsRunningAuthentication() const;

private:
    std::shared_ptr<configuration::AppConfig> appConfig;
    std::shared_ptr<configuration::UserConfig> userConfig;
    std::atomic_bool running_token = ATOMIC_VAR_INIT(false);

    void DoAuthenticationFlow();
    bool WaitForResult(const std::future<std::optional<std::string>> &future) const;
    [[nodiscard]] std::string GetAuthorizeUrl(const std::string& code_challenge, const std::string& client_id, const std::string& redirect_uri) const;

    static void OpenBrowser(const std::string& url);

    std::thread token_thread;

    std::string GetTokenFromAuthorizationCode(const std::string& clientId, const std::string &codeVerifier, const std::string &code, const std::string &redirectUrl) const;
    static size_t WriteCallback(void *contents, size_t size, size_t nmemb, void *userp);

    static std::string generateCodeVerifier();
    static std::string base64_encode(const std::string & in);
    static std::string generateCodeChallenge(const std::string& codeVerifier);
};

}
